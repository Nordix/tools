// Copyright 2019 Istio Authors
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.

package generators

import (
	"io"
	"slices"
	"strings"

	"k8s.io/gengo/generator"
	"k8s.io/gengo/namer"
	"k8s.io/gengo/types"

	"istio.io/tools/cmd/kubetype-gen/metadata"
)

type typesGenerator struct {
	generator.DefaultGen
	source  metadata.PackageMetadata
	imports namer.ImportTracker
}

// NewTypesGenerator creates a new generator for creating k8s style types.go files
func NewTypesGenerator(source metadata.PackageMetadata) generator.Generator {
	return &typesGenerator{
		DefaultGen: generator.DefaultGen{
			OptionalName: "types",
		},
		source:  source,
		imports: generator.NewImportTracker(),
	}
}

func (g *typesGenerator) Namers(c *generator.Context) namer.NameSystems {
	return NameSystems(g.source.TargetPackage().Path, g.imports)
}

func (g *typesGenerator) Imports(c *generator.Context) []string {
	return g.imports.ImportLines()
}

// extracts values for Name and Package from "istiostatus-override" in the comments
func statusOverrideFromComments(commentLines []string) (string, string, bool) {
	// ServiceEntry has a unique status type which includes addresses for auto allocated IPs, substitute IstioServiceEntryStatus
	// for IstioStatus when type is ServiceEntry
	if index := slices.IndexFunc(commentLines, func(comment string) bool {
		return strings.Contains(comment, istioStatusOveride)
	}); index != -1 {
		statusOverrideLine := commentLines[index]
		statusOverridSplit := strings.Split(statusOverrideLine, ":")
		if len(statusOverridSplit) == 2 {
			overrideName := statusOverridSplit[1]
			return strings.TrimSpace(overrideName), "istio.io/api/meta/v1alpha1", true
		} else if len(statusOverridSplit) == 3 {
			overrideName := statusOverridSplit[1]
			overridePackage := statusOverridSplit[2]
			return strings.TrimSpace(overrideName), strings.TrimSpace(overridePackage), true
		}
	}
	return "", "", false
}

func (g *typesGenerator) GenerateType(c *generator.Context, t *types.Type, w io.Writer) error {
	kubeTypes := g.source.KubeTypes(t)
	sw := generator.NewSnippetWriter(w, c, "$", "$")
	m := map[string]interface{}{
		"KubeType":    nil,
		"RawType":     t,
		"TypeMeta":    c.Universe.Type(types.Name{Name: "TypeMeta", Package: "k8s.io/apimachinery/pkg/apis/meta/v1"}),
		"ObjectMeta":  c.Universe.Type(types.Name{Name: "ObjectMeta", Package: "k8s.io/apimachinery/pkg/apis/meta/v1"}),
		"ListMeta":    c.Universe.Type(types.Name{Name: "ListMeta", Package: "k8s.io/apimachinery/pkg/apis/meta/v1"}),
		"IstioStatus": c.Universe.Type(types.Name{Name: "IstioStatus", Package: "istio.io/api/meta/v1alpha1"}),
	}
	for _, kubeType := range kubeTypes {
		localM := m
		// name, package, found := typeFromComments(kubeType.RawType().CommentLines)
		if name, packageName, found := statusOverrideFromComments(kubeType.RawType().CommentLines); found {
			localM["IstioStatus"] = c.Universe.Type(types.Name{Name: name, Package: packageName})
		}

		// make sure local types get imports generated for them to prevent reusing their local name for real imports,
		// e.g. generating into package v1alpha1, while also importing from another package ending with v1alpha1.
		// adding the import here will ensure the imports will be something like, precedingpathv1alpha1.
		g.imports.AddType(kubeType.Type())
		localM["KubeType"] = kubeType
		sw.Do(kubeTypeTemplate, localM)
	}
	return sw.Error()
}

const (
	istioStatusOveride = `istiostatus-override:`
	kubeTypeTemplate   = `
$- range .RawType.SecondClosestCommentLines $
// $ . $
$- end $
$- range .KubeType.Tags $
// +$ . $
$- end $
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

$ range .RawType.CommentLines $
// $ . $
$- end $
type $.KubeType.Type|public$ struct {
	$.TypeMeta|raw$ ` + "`" + `json:",inline"` + "`" + `
	// +optional
	$.ObjectMeta|raw$ ` + "`" + `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"` + "`" + `

	// Spec defines the implementation of this definition.
	// +optional
	Spec $.RawType|raw$ ` + "`" + `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"` + "`" + `

	Status $.IstioStatus|raw$ ` + "`" + `json:"status,omitempty"` + "`" + `
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// $.KubeType.Type|public$List is a collection of $.KubeType.Type|publicPlural$.
type $.KubeType.Type|public$List struct {
	$.TypeMeta|raw$ ` + "`" + `json:",inline"` + "`" + `
	// +optional
	$.ListMeta|raw$ ` + "`" + `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"` + "`" + `
	Items           []*$.KubeType.Type|raw$ ` + "`" + `json:"items" protobuf:"bytes,2,rep,name=items"` + "`" + `
}
`
)
