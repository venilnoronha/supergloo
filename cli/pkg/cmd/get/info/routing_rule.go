package info

import (
	"fmt"
	"strings"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/supergloo/cli/pkg/common"
	glooV1 "github.com/solo-io/supergloo/pkg/api/external/gloo/v1"
	"github.com/solo-io/supergloo/pkg/api/v1"
)

const (
	name         = "NAME"
	targetMesh   = "TARGET-MESH"
	sources      = "SOURCES"
	destinations = "DESTINATIONS"
	matchers     = "MATCHERS"
)

// TODO: ideally at some point we might annotate our .proto files with this information
var routingRuleHeaders = []Header{
	{Name: name, WideOnly: false},
	{Name: targetMesh, WideOnly: false},
	{Name: sources, WideOnly: true},
	{Name: destinations, WideOnly: true},
	{Name: matchers, WideOnly: true},
}

func FromRoutingRule(routingRule *v1.RoutingRule) *ResourceInfo {
	data := transform(routingRule)
	return &ResourceInfo{headers: routingRuleHeaders, data: []map[string]string{data}}
}

func FromRoutingRuleList(list *v1.RoutingRuleList) *ResourceInfo {
	var data Data = make([]map[string]string, 0)
	for _, mesh := range *list {
		data = append(data, transform(mesh))
	}
	return &ResourceInfo{headers: routingRuleHeaders, data: data}
}

func transform(routingRule *v1.RoutingRule) map[string]string {
	var fieldMap = make(map[string]string, len(routingRuleHeaders))
	fieldMap[name] = routingRule.Metadata.Name
	fieldMap[targetMesh] = routingRule.TargetMesh.Name
	fieldMap[sources] = getUpstreams(routingRule.Sources)
	fieldMap[destinations] = getUpstreams(routingRule.Destinations)
	fieldMap[matchers] = getMatchers(routingRule.RequestMatchers)
	return fieldMap
}

func getUpstreams(refs []*core.ResourceRef) string {
	var b strings.Builder
	for i, ref := range refs {
		fmt.Fprintf(&b, "%s%s%s", ref.Namespace, common.NamespacedResourceSeparator, ref.Name)
		// Add separator, except for last entry
		if i != len(refs)-1 {
			b.WriteString(common.ListOptionSeparator)
		}
	}
	return b.String()
}

func getMatchers(matchers []*glooV1.Matcher) string {
	result := make([]string, len(matchers))
	for i, m := range matchers {

		clauses := make([]string, 0)
		if m.PathSpecifier != nil {
			switch specifier := m.PathSpecifier.(type) {
			case *glooV1.Matcher_Prefix:
				clauses = append(clauses, fmt.Sprintf("prefix=%s", specifier.Prefix))
			default:
				//TODO: ignore path specifiers that we currently don't support
			}
		}

		if m.Methods != nil {
			clauses = append(clauses, fmt.Sprintf("methods=%s", strings.Join(m.Methods, common.SubListOptionSeparator)))
		}

		result[i] = strings.Join(clauses, common.ListOptionSeparator)
	}
	return strings.Join(result, " && ")
}
