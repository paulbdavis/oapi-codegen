// Copyright 2019 DeepMap, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package codegen

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"
)

var pathParamRE *regexp.Regexp

func init() {
	pathParamRE = regexp.MustCompile("{[.;?]?([^{}*]+)\\*?}")
}

// Uppercase the first character in a string. This assumes UTF-8, so we have
// to be careful with unicode, don't treat it as a byte array.
func UppercaseFirstCharacter(str string) string {
	if str == "" {
		return ""
	}
	runes := []rune(str)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// Same as above, except lower case
func LowercaseFirstCharacter(str string) string {
	if str == "" {
		return ""
	}
	runes := []rune(str)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

// This function will convert query-arg style strings to CamelCase. We will
// use `., -, +, :, ;, _, ~, ' ', (, ), {, }, [, ]` as valid delimiters for words.
// So, "word.word-word+word:word;word_word~word word(word)word{word}[word]"
// would be converted to WordWordWordWordWordWordWordWordWordWordWordWordWord
func ToPascalCase(str string) string {
	return toCamelorPascalCase(str, true)
}
func ToCamelCase(str string) string {
	return toCamelorPascalCase(str, false)
}

func toCamelorPascalCase(str string, capFirst bool) string {
	separators := "-#@!$&=.+:;_~ (){}[]"
	s := strings.Trim(str, " ")

	n := ""
	capNext := capFirst
	for _, v := range s {
		if unicode.IsUpper(v) {
			n += string(v)
		}
		if unicode.IsDigit(v) {
			n += string(v)
		}
		if unicode.IsLower(v) {
			if capNext {
				n += strings.ToUpper(string(v))
			} else {
				n += string(v)
			}
		}

		if strings.ContainsRune(separators, v) {
			capNext = true
		} else {
			capNext = false
		}
	}
	return fixCamelCaseAbbrev(n)
}

var commonInitialisms = map[string]*regexp.Regexp{
	"ACL":    regexp.MustCompile("Acl([^a-z]+|$)"),
	"API":    regexp.MustCompile("Api([^a-z]+|$)"),
	"ASCII":  regexp.MustCompile("Ascii([^a-z]+|$)"),
	"CPU":    regexp.MustCompile("Cpu([^a-z]+|$)"),
	"CSS":    regexp.MustCompile("Css([^a-z]+|$)"),
	"DNS":    regexp.MustCompile("Dns([^a-z]+|$)"),
	"EOF":    regexp.MustCompile("Eof([^a-z]+|$)"),
	"GUID":   regexp.MustCompile("Guid([^a-z]+|$)"),
	"HTML":   regexp.MustCompile("Html([^a-z]+|$)"),
	"HTTP":   regexp.MustCompile("Http([^a-z]+|$)"),
	"HTTPS":  regexp.MustCompile("Https([^a-z]+|$)"),
	"ID":     regexp.MustCompile("Id([^a-z]+|$)"),
	"IP":     regexp.MustCompile("Ip([^a-z]+|$)"),
	"JSON":   regexp.MustCompile("Json([^a-z]+|$)"),
	"LHS":    regexp.MustCompile("Lhs([^a-z]+|$)"),
	"QPS":    regexp.MustCompile("Qps([^a-z]+|$)"),
	"RAM":    regexp.MustCompile("Ram([^a-z]+|$)"),
	"RHS":    regexp.MustCompile("Rhs([^a-z]+|$)"),
	"RPC":    regexp.MustCompile("Rpc([^a-z]+|$)"),
	"SLA":    regexp.MustCompile("Sla([^a-z]+|$)"),
	"SMTP":   regexp.MustCompile("Smtp([^a-z]+|$)"),
	"SQL":    regexp.MustCompile("Sql([^a-z]+|$)"),
	"SSH":    regexp.MustCompile("Ssh([^a-z]+|$)"),
	"TCP":    regexp.MustCompile("Tcp([^a-z]+|$)"),
	"TLS":    regexp.MustCompile("Tls([^a-z]+|$)"),
	"TTL":    regexp.MustCompile("Ttl([^a-z]+|$)"),
	"UDP":    regexp.MustCompile("Udp([^a-z]+|$)"),
	"UI":     regexp.MustCompile("Ui([^a-z]+|$)"),
	"UID":    regexp.MustCompile("Uid([^a-z]+|$)"),
	"UUID":   regexp.MustCompile("Uuid([^a-z]+|$)"),
	"URI":    regexp.MustCompile("Uri([^a-z]+|$)"),
	"URL":    regexp.MustCompile("Url([^a-z]+|$)"),
	"UTF8":   regexp.MustCompile("Ut([^a-z]+|$)"),
	"VM":     regexp.MustCompile("Vm([^a-z]+|$)"),
	"XML":    regexp.MustCompile("Xml([^a-z]+|$)"),
	"XMPP":   regexp.MustCompile("Xmpp([^a-z]+|$)"),
	"XSRF":   regexp.MustCompile("Xsrf([^a-z]+|$)"),
	"XSS":    regexp.MustCompile("Xss([^a-z]+|$)"),
	"OAuth":  regexp.MustCompile("Oauth([^a-z]+|$)"),
	"OFAC":   regexp.MustCompile("Ofac([^a-z]+|$)"),
	"NASA":   regexp.MustCompile("Nasa([^a-z]+|$)"),
	"USD":    regexp.MustCompile("Usd([^a-z]+|$)"),
	"EUR":    regexp.MustCompile("Eur([^a-z]+|$)"),
	"BTC":    regexp.MustCompile("Btc([^a-z]+|$)"),
	"ETH":    regexp.MustCompile("Eth([^a-z]+|$)"),
	"PDF":    regexp.MustCompile("Pdf([^a-z]+|$)"),
	"PDF417": regexp.MustCompile("Pdf417([^a-z]+|$)"),
	"SSN":    regexp.MustCompile("Ssn([^a-z]+|$)"),
	"SMS":    regexp.MustCompile("Sms([^a-z]+|$)"),
}

func fixCamelCaseAbbrev(str string) string {
	for rep, re := range commonInitialisms {
		str = re.ReplaceAllString(str, fmt.Sprintf("%s$1", rep))
	}
	return str
}

type keySlice []string

func (keys keySlice) Len() int {
	return len(keys)
}

func (keys keySlice) Less(i, j int) bool {
	iKey := keys[i]
	jKey := keys[j]

	if strings.EqualFold(iKey, "id") {
		return true
	}
	if strings.EqualFold(jKey, "id") {
		return false
	}
	if len(iKey) <= 3 || len(jKey) <= 3 {
		return iKey < jKey
	}

	iAt := iKey[len(iKey)-3:] == "_at"
	jAt := jKey[len(jKey)-3:] == "_at"

	if iAt && jAt {
		return iKey < jKey
	} else if iAt {
		return false
	} else if jAt {
		return true
	}

	return iKey < jKey
}

func (keys keySlice) Swap(i, j int) {
	tmpKey := keys[i]
	keys[i] = keys[j]
	keys[j] = tmpKey
}

// This function returns the keys of the given SchemaRef dictionary in sorted
// order, since Golang scrambles dictionary keys
func SortedSchemaKeys(dict map[string]*openapi3.SchemaRef) []string {
	keys := make([]string, len(dict))
	i := 0
	for key := range dict {
		keys[i] = key
		i++
	}
	sort.Stable(keySlice(keys))
	return keys
}

// This function is the same as above, except it sorts the keys for a Paths
// dictionary.
func SortedPathsKeys(dict openapi3.Paths) []string {
	keys := make([]string, len(dict))
	i := 0
	for key := range dict {
		keys[i] = key
		i++
	}
	sort.Stable(keySlice(keys))
	return keys
}

// This function returns Operation dictionary keys in sorted order
func SortedOperationsKeys(dict map[string]*openapi3.Operation) []string {
	keys := make([]string, len(dict))
	i := 0
	for key := range dict {
		keys[i] = key
		i++
	}
	sort.Stable(keySlice(keys))
	return keys
}

// This function returns Responses dictionary keys in sorted order
func SortedResponsesKeys(dict openapi3.Responses) []string {
	keys := make([]string, len(dict))
	i := 0
	for key := range dict {
		keys[i] = key
		i++
	}
	sort.Stable(keySlice(keys))
	return keys
}

// This returns Content dictionary keys in sorted order
func SortedContentKeys(dict openapi3.Content) []string {
	keys := make([]string, len(dict))
	i := 0
	for key := range dict {
		keys[i] = key
		i++
	}
	sort.Stable(keySlice(keys))
	return keys
}

// This returns string map keys in sorted order
func SortedStringKeys(dict map[string]string) []string {
	keys := make([]string, len(dict))
	i := 0
	for key := range dict {
		keys[i] = key
		i++
	}
	sort.Stable(keySlice(keys))
	return keys
}

// This returns sorted keys for a ParameterRef dict
func SortedParameterKeys(dict map[string]*openapi3.ParameterRef) []string {
	keys := make([]string, len(dict))
	i := 0
	for key := range dict {
		keys[i] = key
		i++
	}
	sort.Stable(keySlice(keys))
	return keys
}

func SortedRequestBodyKeys(dict map[string]*openapi3.RequestBodyRef) []string {
	keys := make([]string, len(dict))
	i := 0
	for key := range dict {
		keys[i] = key
		i++
	}
	sort.Stable(keySlice(keys))
	return keys
}

func SortedSecurityRequirementKeys(sr openapi3.SecurityRequirement) []string {
	keys := make([]string, len(sr))
	i := 0
	for key := range sr {
		keys[i] = key
		i++
	}
	sort.Stable(keySlice(keys))
	return keys
}

// This function checks whether the specified string is present in an array
// of strings
func StringInArray(str string, array []string) bool {
	for _, elt := range array {
		if elt == str {
			return true
		}
	}
	return false
}

// This function takes a $ref value and converts it to a Go typename.
// #/components/schemas/Foo -> Foo
// #/components/parameters/Bar -> Bar
// #/components/responses/Baz -> Baz
// Remote components (document.json#/Foo) are supported if they present in --import-mapping
// URL components (http://deepmap.com/schemas/document.json#/Foo) are supported if they present in --import-mapping
// Remote and URL also support standard local paths even though the spec doesn't mention them.
func RefPathToGoType(refPath string) (string, error) {
	return refPathToGoType(refPath, true)
}

// refPathToGoType returns the Go typename for refPath given its
func refPathToGoType(refPath string, local bool) (string, error) {
	if refPath[0] == '#' {
		pathParts := strings.Split(refPath, "/")
		depth := len(pathParts)
		if local {
			if depth != 4 {
				return "", fmt.Errorf("unexpected reference depth: %d for ref: %s local: %t", depth, refPath, local)
			}
		} else if depth != 4 && depth != 2 {
			return "", fmt.Errorf("unexpected reference depth: %d for ref: %s local: %t", depth, refPath, local)
		}
		return SchemaNameToTypeName(pathParts[len(pathParts)-1]), nil
	}
	pathParts := strings.Split(refPath, "#")
	if len(pathParts) != 2 {
		return "", fmt.Errorf("unsupported reference: %s", refPath)
	}
	remoteComponent, flatComponent := pathParts[0], pathParts[1]
	if goImport, ok := importMapping[remoteComponent]; !ok {
		return "", fmt.Errorf("unrecognized external reference '%s'; please provide the known import for this reference using option --import-mapping", remoteComponent)
	} else {
		goType, err := refPathToGoType("#"+flatComponent, false)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.%s", goImport.Name, goType), nil
	}
}

// This function takes a $ref value and checks if it has link to go type.
// #/components/schemas/Foo                     -> true
// ./local/file.yml#/components/parameters/Bar  -> true
// ./local/file.yml                             -> false
// The function can be used to check whether RefPathToGoType($ref) is possible.
//
func IsGoTypeReference(ref string) bool {
	return ref != "" && !IsWholeDocumentReference(ref)
}

// This function takes a $ref value and checks if it is whole document reference.
// #/components/schemas/Foo                             -> false
// ./local/file.yml#/components/parameters/Bar          -> false
// ./local/file.yml                                     -> true
// http://deepmap.com/schemas/document.json             -> true
// http://deepmap.com/schemas/document.json#/Foo        -> false
//
func IsWholeDocumentReference(ref string) bool {
	return ref != "" && !strings.ContainsAny(ref, "#")
}

// This function converts a swagger style path URI with parameters to a
// Echo compatible path URI. We need to replace all of Swagger parameters with
// ":param". Valid input parameters are:
//   {param}
//   {param*}
//   {.param}
//   {.param*}
//   {;param}
//   {;param*}
//   {?param}
//   {?param*}
func SwaggerUriToEchoUri(uri string) string {
	return pathParamRE.ReplaceAllString(uri, ":$1")
}

// This function converts a swagger style path URI with parameters to a
// Chi compatible path URI. We need to replace all of Swagger parameters with
// "{param}". Valid input parameters are:
//   {param}
//   {param*}
//   {.param}
//   {.param*}
//   {;param}
//   {;param*}
//   {?param}
//   {?param*}
func SwaggerUriToChiUri(uri string) string {
	return pathParamRE.ReplaceAllString(uri, "{$1}")
}

// This function converts a swagger style path URI with parameters to a
// Gin compatible path URI. We need to replace all of Swagger parameters with
// ":param". Valid input parameters are:
//   {param}
//   {param*}
//   {.param}
//   {.param*}
//   {;param}
//   {;param*}
//   {?param}
//   {?param*}
func SwaggerUriToGinUri(uri string) string {
	return pathParamRE.ReplaceAllString(uri, ":$1")
}

// Returns the argument names, in order, in a given URI string, so for
// /path/{param1}/{.param2*}/{?param3}, it would return param1, param2, param3
func OrderedParamsFromUri(uri string) []string {
	matches := pathParamRE.FindAllStringSubmatch(uri, -1)
	result := make([]string, len(matches))
	for i, m := range matches {
		result[i] = m[1]
	}
	return result
}

// Replaces path parameters of the form {param} with %s
func ReplacePathParamsWithStr(uri string) string {
	return pathParamRE.ReplaceAllString(uri, "%s")
}

// Reorders the given parameter definitions to match those in the path URI.
func SortParamsByPath(path string, in []ParameterDefinition) ([]ParameterDefinition, error) {
	pathParams := OrderedParamsFromUri(path)
	n := len(in)
	if len(pathParams) != n {
		return nil, fmt.Errorf("path '%s' has %d positional parameters, but spec has %d declared",
			path, len(pathParams), n)
	}
	out := make([]ParameterDefinition, len(in))
	for i, name := range pathParams {
		p := ParameterDefinitions(in).FindByName(name)
		if p == nil {
			return nil, fmt.Errorf("path '%s' refers to parameter '%s', which doesn't exist in specification",
				path, name)
		}
		out[i] = *p
	}
	return out, nil
}

// Returns whether the given string is a go keyword
func IsGoKeyword(str string) bool {
	keywords := []string{
		"break",
		"case",
		"chan",
		"const",
		"continue",
		"default",
		"defer",
		"else",
		"fallthrough",
		"for",
		"func",
		"go",
		"goto",
		"if",
		"import",
		"interface",
		"map",
		"package",
		"range",
		"return",
		"select",
		"struct",
		"switch",
		"type",
		"var",
	}

	for _, k := range keywords {
		if k == str {
			return true
		}
	}
	return false
}

// IsPredeclaredGoIdentifier returns whether the given string
// is a predefined go indentifier.
//
// See https://golang.org/ref/spec#Predeclared_identifiers
func IsPredeclaredGoIdentifier(str string) bool {
	predeclaredIdentifiers := []string{
		// Types
		"bool",
		"byte",
		"complex64",
		"complex128",
		"error",
		"float32",
		"float64",
		"int",
		"int8",
		"int16",
		"int32",
		"int64",
		"rune",
		"string",
		"uint",
		"uint8",
		"uint16",
		"uint32",
		"uint64",
		"uintptr",
		// Constants
		"true",
		"false",
		"iota",
		// Zero value
		"nil",
		// Functions
		"append",
		"cap",
		"close",
		"complex",
		"copy",
		"delete",
		"imag",
		"len",
		"make",
		"new",
		"panic",
		"print",
		"println",
		"real",
		"recover",
	}

	for _, k := range predeclaredIdentifiers {
		if k == str {
			return true
		}
	}

	return false
}

// IsGoIdentity checks if the given string can be used as an identity
// in the generated code like a type name or constant name.
//
// See https://golang.org/ref/spec#Identifiers
func IsGoIdentity(str string) bool {
	for i, c := range str {
		if !isValidRuneForGoID(i, c) {
			return false
		}
	}

	return IsGoKeyword(str)
}

func isValidRuneForGoID(index int, char rune) bool {
	if index == 0 && unicode.IsNumber(char) {
		return false
	}

	return unicode.IsLetter(char) || char == '_' || unicode.IsNumber(char)
}

// IsValidGoIdentity checks if the given string can be used as a
// name of variable, constant, or type.
func IsValidGoIdentity(str string) bool {
	if IsGoIdentity(str) {
		return false
	}

	return !IsPredeclaredGoIdentifier(str)
}

// SanitizeGoIdentity deletes and replaces the illegal runes in the given
// string to use the string as a valid identity.
func SanitizeGoIdentity(str string) string {
	sanitized := []rune(str)

	for i, c := range sanitized {
		if !isValidRuneForGoID(i, c) {
			sanitized[i] = '_'
		} else {
			sanitized[i] = c
		}
	}

	str = string(sanitized)

	if IsGoKeyword(str) || IsPredeclaredGoIdentifier(str) {
		str = "_" + str
	}

	if !IsValidGoIdentity(str) {
		panic("here is a bug")
	}

	return str
}

// SanitizeEnumNames fixes illegal chars in the enum names
// and removes duplicates
func SanitizeEnumNames(enumNames []string) map[string]string {
	dupCheck := make(map[string]int, len(enumNames))
	deDup := make([]string, 0, len(enumNames))

	for _, n := range enumNames {
		if _, dup := dupCheck[n]; !dup {
			deDup = append(deDup, n)
		}
		dupCheck[n] = 0
	}

	dupCheck = make(map[string]int, len(deDup))
	sanitizedDeDup := make(map[string]string, len(deDup))

	var hasMultiWord bool
	for _, n := range deDup {
		if strings.ContainsAny(n, "_- ") {
			hasMultiWord = true
		}
	}

	for _, n := range deDup {
		toSanitize := n
		if hasMultiWord {
			toSanitize = strings.ToLower(n)
		}
		sanitized := SanitizeGoIdentity(SchemaNameToEnumValueName(toSanitize))

		if _, dup := dupCheck[sanitized]; !dup {
			sanitizedDeDup[sanitized] = n
		} else {
			sanitizedDeDup[sanitized+strconv.Itoa(dupCheck[sanitized])] = n
		}
		dupCheck[sanitized]++
	}

	return sanitizedDeDup
}

func typeNamePrefix(name string) (prefix string) {
	for _, r := range name {
		switch r {
		case '$':
			if len(name) == 1 {
				return "DollarSign"
			}
		case '-':
			prefix += "Minus"
		case '+':
			prefix += "Plus"
		case '&':
			prefix += "And"
		case '~':
			prefix += "Tilde"
		case '=':
			prefix += "Equal"
		case '#':
			prefix += "Hash"
		case '.':
			prefix += "Dot"
		default:
			// Prepend "N" to schemas starting with a number
			if prefix == "" && unicode.IsDigit(r) {
				return "N"
			}

			// break the loop, done parsing prefix
			return
		}
	}

	return
}

// SchemaNameToTypeName converts a Schema name to a valid Go type name. It converts to camel case, and makes sure the name is
// valid in Go

func SchemaNameToTypeName(name string) string {
	return typeNamePrefix(name) + ToPascalCase(name)
}

func SchemaNameToEnumValueName(name string) string {
	return typeNamePrefix(name) + ToPascalCase(strings.ReplaceAll(name, "_", "-"))
}

// According to the spec, additionalProperties may be true, false, or a
// schema. If not present, true is implied. If it's a schema, true is implied.
// If it's false, no additional properties are allowed. We're going to act a little
// differently, in that if you want additionalProperties code to be generated,
// you must specify an additionalProperties type
// If additionalProperties it true/false, this field will be non-nil.
func SchemaHasAdditionalProperties(schema *openapi3.Schema) bool {
	if schema.AdditionalPropertiesAllowed != nil && *schema.AdditionalPropertiesAllowed {
		return true
	}

	if schema.AdditionalProperties != nil {
		return true
	}
	return false
}

// This converts a path, like Object/field1/nestedField into a go
// type name.
func PathToTypeName(path []string) string {
	for i, p := range path {
		path[i] = ToPascalCase(p)
	}
	return strings.Join(path, "_")
}

// StringToGoComment renders a possible multi-line string as a valid Go-Comment.
// Each line is prefixed as a comment.
func StringToGoComment(in string) string {
	if len(in) == 0 || len(strings.TrimSpace(in)) == 0 { // ignore empty comment
		return ""
	}

	// Normalize newlines from Windows/Mac to Linux
	in = strings.Replace(in, "\r\n", "\n", -1)
	in = strings.Replace(in, "\r", "\n", -1)

	// Add comment to each line
	var lines []string
	for _, line := range strings.Split(in, "\n") {
		lines = append(lines, fmt.Sprintf("// %s", line))
	}
	in = strings.Join(lines, "\n")

	// in case we have a multiline string which ends with \n, we would generate
	// empty-line-comments, like `// `. Therefore remove this line comment.
	in = strings.TrimSuffix(in, "\n// ")
	return in
}

// This function breaks apart a path, and looks at each element. If it's
// not a path parameter, eg, {param}, it will URL-escape the element.
func EscapePathElements(path string) string {
	elems := strings.Split(path, "/")
	for i, e := range elems {
		if strings.HasPrefix(e, "{") && strings.HasSuffix(e, "}") {
			// This is a path parameter, we don't want to mess with its value
			continue
		}
		elems[i] = url.QueryEscape(e)
	}
	return strings.Join(elems, "/")
}
