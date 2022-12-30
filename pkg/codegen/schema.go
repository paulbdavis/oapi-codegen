package codegen

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// This describes a Schema, a type definition.
type Schema struct {
	GoType    string // The Go type needed to represent the schema
	RefType   string // If the type has a type name, this is set
	RefGoType string // The Go type of the ref type

	ArrayType *Schema // The schema of array element

	EnumValues map[string]string // Enum values

	Properties               []Property       // For an object, the fields with names
	HasAdditionalProperties  bool             // Whether we support additional properties
	AdditionalPropertiesType *Schema          // And if we do, their type
	AdditionalTypes          []TypeDefinition // We may need to generate auxiliary helper types, stored here

	SkipOptionalPointer bool // Some types don't need a * in front when they're optional

	Description string // The description of the element

	// The original OpenAPIv3 Schema.
	OAPISchema *openapi3.Schema
}

func (s Schema) IsRef() bool {
	return s.RefType != ""
}

func (s Schema) TypeDecl() string {
	if s.IsRef() {
		return s.RefType
	}
	return s.GoType
}

// AddProperty adds a new property to the current Schema, and returns an error
// if it collides. Two identical fields will not collide, but two properties by
// the same name, but different definition, will collide. It's safe to merge the
// fields of two schemas with overalapping properties if those properties are
// identical.
func (s *Schema) AddProperty(p Property) error {
	// Scan all existing properties for a conflict
	for _, e := range s.Properties {
		if e.JsonFieldName == p.JsonFieldName && !PropertiesEqual(e, p) {
			return errors.New(fmt.Sprintf("property '%s' already exists with a different type", e.JsonFieldName))
		}
	}
	s.Properties = append(s.Properties, p)
	return nil
}

func (s Schema) GetAdditionalTypeDefs() []TypeDefinition {
	var result []TypeDefinition
	for _, p := range s.Properties {
		result = append(result, p.Schema.GetAdditionalTypeDefs()...)
	}
	result = append(result, s.AdditionalTypes...)
	return result
}

type Property struct {
	Description    string
	JsonFieldName  string
	Schema         Schema
	Required       bool
	Nullable       bool
	ReadOnly       bool
	WriteOnly      bool
	ExtensionProps *openapi3.ExtensionProps
}

func (p Property) GoFieldName() string {
	return SchemaNameToTypeName(p.JsonFieldName)
}

func (p Property) GoTypeDef() string {
	typeDef := p.Schema.TypeDecl()
	if !p.Schema.SkipOptionalPointer &&
		(!p.Required || p.Nullable || p.ReadOnly || p.WriteOnly) {

		typeDef = "*" + typeDef
	}
	return typeDef
}

// EnumDefinition holds type information for enum
type EnumDefinition struct {
	Schema       Schema
	TypeName     string
	ValueWrapper string
}

type Constants struct {
	// SecuritySchemeProviderNames holds all provider names for security schemes.
	SecuritySchemeProviderNames []string
	// EnumDefinitions holds type and value information for all enums
	EnumDefinitions []EnumDefinition
}

// TypeDefinition describes a Go type definition in generated code.
//
// Let's use this example schema:
// components:
//
//	schemas:
//	  Person:
//	    type: object
//	    properties:
//	    name:
//	      type: string
type TypeDefinition struct {
	// The name of the type, eg, type <...> Person
	TypeName string

	// The name of the corresponding JSON description, as it will sometimes
	// differ due to invalid characters.
	JsonName string

	// This is the Schema wrapper is used to populate the type description
	Schema Schema
}

// ResponseTypeDefinition is an extension of TypeDefinition, specifically for
// response unmarshaling in ClientWithResponses.
type ResponseTypeDefinition struct {
	TypeDefinition
	// The content type name where this is used, eg, application/json
	ContentTypeName string

	// The type name of a response model.
	ResponseName string
}

func (t *TypeDefinition) CanAlias() bool {
	return t.Schema.IsRef() || /* actual reference */
		(t.Schema.ArrayType != nil && t.Schema.ArrayType.IsRef()) /* array to ref */
}

func PropertiesEqual(a, b Property) bool {
	return a.JsonFieldName == b.JsonFieldName && a.Schema.TypeDecl() == b.Schema.TypeDecl() && a.Required == b.Required
}

func generateProperties(schema *openapi3.Schema, path []string, outSchema Schema) (Schema, error) {

	// We've got an object with some properties.
	for _, pName := range SortedSchemaKeys(schema.Properties) {
		p := schema.Properties[pName]
		propertyPath := append(path, pName)
		pSchema, err := GenerateGoSchema(p, propertyPath)
		if err != nil {
			return Schema{}, fmt.Errorf("error generating Go schema for property '%s': %w", pName, err)
		}
		// log.Printf("generated go schema for property '%s': %#v", pName, pSchema)

		required := StringInArray(pName, schema.Required)

		if pSchema.HasAdditionalProperties && pSchema.RefType == "" {
			// If we have fields present which have additional properties,
			// but are not a pre-defined type, we need to define a type
			// for them, which will be based on the field names we followed
			// to get to the type.
			typeName := PathToTypeName(propertyPath)

			typeDef := TypeDefinition{
				TypeName: typeName,
				JsonName: strings.Join(propertyPath, "."),
				Schema:   pSchema,
			}
			pSchema.AdditionalTypes = append(pSchema.AdditionalTypes, typeDef)

			pSchema.RefType = typeName
		}
		description := ""
		if p.Value != nil {
			description = p.Value.Description
		}
		prop := Property{
			JsonFieldName:  pName,
			Schema:         pSchema,
			Required:       required,
			Description:    description,
			Nullable:       p.Value.Nullable,
			ReadOnly:       p.Value.ReadOnly,
			WriteOnly:      p.Value.WriteOnly,
			ExtensionProps: &p.Value.ExtensionProps,
		}
		outSchema.Properties = append(outSchema.Properties, prop)
	}

	outSchema.HasAdditionalProperties = SchemaHasAdditionalProperties(schema)
	outSchema.AdditionalPropertiesType = &Schema{
		GoType: "interface{}",
	}
	if schema.AdditionalProperties != nil {
		additionalSchema, err := GenerateGoSchema(schema.AdditionalProperties, path)
		if err != nil {
			return Schema{}, fmt.Errorf("error generating type for additional properties: %w", err)
		}
		outSchema.AdditionalPropertiesType = &additionalSchema
	}

	return outSchema, nil
}

func generateEnumValues(schema *openapi3.Schema, path []string, outSchema Schema) (Schema, error) {
	if len(schema.Enum) == 0 {
		return outSchema, nil
	}
	enumValues := make([]string, len(schema.Enum))
	for i, enumValue := range schema.Enum {
		enumValues[i] = fmt.Sprintf("%v", enumValue)
	}

	sanitizedValues := SanitizeEnumNames(enumValues)
	outSchema.EnumValues = make(map[string]string, len(sanitizedValues))
	var constNamePath []string
	for k, v := range sanitizedValues {
		if v == "" {
			constNamePath = append(path, "Empty")
		} else {
			constNamePath = append(path, k)
		}
		outSchema.EnumValues[SchemaNameToEnumValueName(PathToTypeName(constNamePath))] = v
	}
	if len(path) > 1 { // handle additional type only on non-toplevel types
		typeName := SchemaNameToTypeName(PathToTypeName(path))
		typeDef := TypeDefinition{
			TypeName: typeName,
			JsonName: strings.Join(path, "."),
			Schema:   outSchema,
		}
		outSchema.AdditionalTypes = append(outSchema.AdditionalTypes, typeDef)
		outSchema.RefType = typeName
	}
	//outSchema.RefType = typeName

	return outSchema, nil

}

func GenerateGoSchema(sref *openapi3.SchemaRef, path []string) (Schema, error) {
	// Add a fallback value in case the sref is nil.
	// i.e. the parent schema defines a type:array, but the array has
	// no items defined. Therefore we have at least valid Go-Code.
	if sref == nil {
		return Schema{GoType: "interface{}"}, nil
	}

	schema := sref.Value

	// If Ref is set on the SchemaRef, it means that this type is actually a reference to
	// another type. We're not de-referencing, so simply use the referenced type.
	if IsGoTypeReference(sref.Ref) {
		// Convert the reference path to Go type
		refType, err := RefPathToGoType(sref.Ref)
		if err != nil {
			return Schema{}, fmt.Errorf("error turning reference (%s) into a Go type: %s",
				sref.Ref, err)
		}

		schemas := []*openapi3.Schema{schema}

		if schema.AllOf != nil {
			for _, sRef := range schema.AllOf {
				schemas = append(schemas, sRef.Value)
			}
		}

		outSchema := Schema{
			GoType:      refType,
			Description: StringToGoComment(schema.Description),
		}

		outSchema.RefGoType, err = resolveGoType(schema.Format, schema.Type)
		if err != nil {
			log.Println("error resolving go type for ref:", sref.Ref)
		}

		for _, scm := range schemas {
			new, err := generateProperties(scm, path, outSchema)
			if err != nil {
				return Schema{}, err
			}
			new, err = generateEnumValues(scm, path, new)
			if err != nil {
				return Schema{}, err
			}

			outSchema.Properties = append(outSchema.Properties, new.Properties...)
			for k, v := range new.EnumValues {
				if outSchema.EnumValues == nil {
					outSchema.EnumValues = map[string]string{}
				}
				outSchema.EnumValues[k] = v
			}
		}

		return outSchema, nil
	}

	outSchema := Schema{
		Description: StringToGoComment(schema.Description),
		OAPISchema:  schema,
	}

	// We can't support this in any meaningful way
	if schema.AnyOf != nil {
		outSchema.GoType = "interface{}"
		return outSchema, nil
	}
	// We can't support this in any meaningful way
	if schema.OneOf != nil {
		outSchema.GoType = "interface{}"
		return outSchema, nil
	}

	// AllOf is interesting, and useful. It's the union of a number of other
	// schemas. A common usage is to create a union of an object with an ID,
	// so that in a RESTful paradigm, the Create operation can return
	// (object, id), so that other operations can refer to (id)
	if schema.AllOf != nil {
		mergedSchema, err := MergeSchemas(schema.AllOf, path)
		if err != nil {
			return Schema{}, fmt.Errorf("error merging schemas: %w", err)
		}
		mergedSchema.OAPISchema = schema
		return mergedSchema, nil
	}

	// Check for custom Go type extension
	if extension, ok := schema.Extensions[extPropGoType]; ok {
		typeName, err := extTypeName(extension)
		if err != nil {
			return outSchema, fmt.Errorf("invalid value for %q: %w", extPropGoType, err)
		}
		outSchema.GoType = typeName
		return outSchema, nil
	}

	// Schema type and format, eg. string / binary
	t := schema.Type
	// Handle objects and empty schemas first as a special case
	if t == "" || t == "object" {
		var outType string

		if len(schema.Properties) == 0 && !SchemaHasAdditionalProperties(schema) {
			// If the object has no properties or additional properties, we
			// have some special cases for its type.
			if t == "object" {
				// We have an object with no properties. This is a generic object
				// expressed as a map.
				outType = "map[string]interface{}"
			} else { // t == ""
				// If we don't even have the object designator, we're a completely
				// generic type.
				outType = "interface{}"
			}
			outSchema.GoType = outType
		} else {

			var err error
			outSchema, err = generateProperties(schema, path, outSchema)
			if err != nil {
				return Schema{}, err
			}

			outSchema.GoType = GenStructFromSchema(outSchema)
		}
		return outSchema, nil
	} else if len(schema.Enum) > 0 {
		err := resolveType(schema, path, &outSchema)
		if err != nil {
			return Schema{}, fmt.Errorf("error resolving primitive type: %w", err)
		}

		outSchema, err = generateEnumValues(schema, path, outSchema)
		if err != nil {
			return Schema{}, err
		}
	} else {
		err := resolveType(schema, path, &outSchema)
		if err != nil {
			return Schema{}, fmt.Errorf("error resolving primitive type: %w", err)
		}
	}
	return outSchema, nil
}

func resolveGoType(f, t string) (string, error) {
	switch t {
	case "array":
		return "array", nil
	case "integer":
		// We default to int if format doesn't ask for something else.
		switch f {
		case "int64":
			return "int64", nil
		case "int32":
			return "int32", nil
		case "int16":
			return "int16", nil
		case "int8":
			return "int8", nil
		case "int":
			return "int", nil
		case "uint64":
			return "uint64", nil
		case "uint32":
			return "uint32", nil
		case "uint16":
			return "uint16", nil
		case "uint8":
			return "uint8", nil
		case "uint":
			return "uint", nil
		case "":
			return "int", nil
		}
		return "", fmt.Errorf("invalid integer format: %s", f)
	case "number":
		// We default to float for "number"
		switch f {
		case "double":
			return "float64", nil
		case "float", "":
			return "float32", nil
		}
		return "", fmt.Errorf("invalid number format: %s", f)
	case "boolean":
		if f != "" {
			return "", fmt.Errorf("invalid format (%s) for boolean", f)
		}
		return "bool", nil
	case "string":
		// Special case string formats here.
		switch f {
		case "byte":
			return "[]byte", nil
		case "email":
			return "openapi_types.Email", nil
		case "date":
			return "openapi_types.Date", nil
		case "date-time":
			return "time.Time", nil
		case "json":
			return "json.RawMessage", nil
		case "uuid":
			return "openapi_types.UUID", nil
		default:
			// All unrecognized formats are simply a regular string.
			return "string", nil
		}
	}

	return "", fmt.Errorf("unhandled Schema type: %s", t)
}

// resolveType resolves primitive  type or array for schema
func resolveType(schema *openapi3.Schema, path []string, outSchema *Schema) error {
	f := schema.Format
	t := schema.Type

	goType, err := resolveGoType(f, t)
	if err != nil {
		return err
	}
	switch goType {
	case "array":
		// For arrays, we'll get the type of the Items and throw a
		// [] in front of it.
		arrayType, err := GenerateGoSchema(schema.Items, path)
		if err != nil {
			return fmt.Errorf("error generating type for array: %w", err)
		}
		outSchema.ArrayType = &arrayType
		outSchema.GoType = "[]" + arrayType.TypeDecl()
		outSchema.AdditionalTypes = arrayType.AdditionalTypes
		outSchema.Properties = arrayType.Properties
	case "json.RawMessage":
		outSchema.SkipOptionalPointer = true
		outSchema.GoType = goType
	default:
		outSchema.GoType = goType

	}
	return nil
}

// This describes a Schema, a type definition.
type SchemaDescriptor struct {
	Fields                   []FieldDescriptor
	HasAdditionalProperties  bool
	AdditionalPropertiesType string
}

type FieldDescriptor struct {
	Required bool   // Is the schema required? If not, we'll pass by pointer
	GoType   string // The Go type needed to represent the json type.
	GoName   string // The Go compatible type name for the type
	JsonName string // The json type name for the type
	IsRef    bool   // Is this schema a reference to predefined object?
}

// Given a list of schema descriptors, produce corresponding field names with
// JSON annotations
func GenFieldsFromProperties(props []Property) []string {
	var fields []string
	for i, p := range props {
		field := ""
		// Add a comment to a field in case we have one, otherwise skip.
		if p.Description != "" {
			// Separate the comment from a previous-defined, unrelated field.
			// Make sure the actual field is separated by a newline.
			if i != 0 {
				field += "\n"
			}
			field += fmt.Sprintf("%s\n", StringToGoComment(p.Description))
		}

		goFieldName := p.GoFieldName()
		if _, ok := p.ExtensionProps.Extensions[extGoFieldName]; ok {
			if extGoFieldName, err := extParseGoFieldName(p.ExtensionProps.Extensions[extGoFieldName]); err == nil {
				goFieldName = extGoFieldName
			}
		}

		field += fmt.Sprintf("    %s %s", goFieldName, p.GoTypeDef())

		// Support x-omitempty
		omitEmpty := true
		if _, ok := p.ExtensionProps.Extensions[extPropOmitEmpty]; ok {
			if extOmitEmpty, err := extParseOmitEmpty(p.ExtensionProps.Extensions[extPropOmitEmpty]); err == nil {
				omitEmpty = extOmitEmpty
			}
		}

		fieldTags := make(map[string]string)

		if (p.Required && !p.ReadOnly && !p.WriteOnly) || p.Nullable || !omitEmpty {
			fieldTags["json"] = p.JsonFieldName
		} else {
			fieldTags["json"] = p.JsonFieldName + ",omitempty"
		}
		if extension, ok := p.ExtensionProps.Extensions[extPropExtraTags]; ok {
			if tags, err := extExtraTags(extension); err == nil {
				keys := SortedStringKeys(tags)
				for _, k := range keys {
					fieldTags[k] = tags[k]
				}
			}
		}
		// Convert the fieldTags map into Go field annotations.
		keys := SortedStringKeys(fieldTags)
		tags := make([]string, len(keys))
		for i, k := range keys {
			tags[i] = fmt.Sprintf(`%s:"%s"`, k, fieldTags[k])
		}
		field += "`" + strings.Join(tags, " ") + "`"
		fields = append(fields, field)
	}
	return fields
}

func GenStructFromSchema(schema Schema) string {
	// Start out with struct {
	objectParts := []string{"struct {"}
	// Append all the field definitions
	objectParts = append(objectParts, GenFieldsFromProperties(schema.Properties)...)
	// Close the struct
	if schema.HasAdditionalProperties {
		addPropsType := schema.AdditionalPropertiesType.GoType
		if schema.AdditionalPropertiesType.RefType != "" {
			addPropsType = schema.AdditionalPropertiesType.RefType
		}

		objectParts = append(objectParts,
			fmt.Sprintf("AdditionalProperties map[string]%s `json:\"-\"`", addPropsType))
	}
	objectParts = append(objectParts, "}")
	return strings.Join(objectParts, "\n")
}

// This constructs a Go type for a parameter, looking at either the schema or
// the content, whichever is available
func paramToGoType(param *openapi3.Parameter, path []string) (Schema, error) {
	if param.Content == nil && param.Schema == nil {
		return Schema{}, fmt.Errorf("parameter '%s' has no schema or content", param.Name)
	}

	// We can process the schema through the generic schema processor
	if param.Schema != nil {
		return GenerateGoSchema(param.Schema, path)
	}

	// At this point, we have a content type. We know how to deal with
	// application/json, but if multiple formats are present, we can't do anything,
	// so we'll return the parameter as a string, not bothering to decode it.
	if len(param.Content) > 1 {
		return Schema{
			GoType:      "string",
			Description: StringToGoComment(param.Description),
		}, nil
	}

	// Otherwise, look for application/json in there
	mt, found := param.Content["application/json"]
	if !found {
		// If we don't have json, it's a string
		return Schema{
			GoType:      "string",
			Description: StringToGoComment(param.Description),
		}, nil
	}

	// For json, we go through the standard schema mechanism
	return GenerateGoSchema(mt.Schema, path)
}
