package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/ojarosch/iacbom/internal/bom"
)

// Minimal valid SPDX 2.3 JSON. Terraform/OpenTofu concepts map to packages
// with NOASSERTION license/download fields rather than being misrepresented.
// The creation date is fixed to keep output deterministic.
const spdxCreated = "1970-01-01T00:00:00Z"

type spdxDoc struct {
	SPDXVersion       string        `json:"spdxVersion"`
	DataLicense       string        `json:"dataLicense"`
	SPDXID            string        `json:"SPDXID"`
	Name              string        `json:"name"`
	DocumentNamespace string        `json:"documentNamespace"`
	CreationInfo      spdxCreation  `json:"creationInfo"`
	Packages          []spdxPackage `json:"packages"`
	Relationships     []spdxRel     `json:"relationships"`
}

type spdxCreation struct {
	Created  string   `json:"created"`
	Creators []string `json:"creators"`
}

type spdxPackage struct {
	Name             string `json:"name"`
	SPDXID           string `json:"SPDXID"`
	VersionInfo      string `json:"versionInfo,omitempty"`
	DownloadLocation string `json:"downloadLocation"`
	FilesAnalyzed    bool   `json:"filesAnalyzed"`
	LicenseConcluded string `json:"licenseConcluded"`
	LicenseDeclared  string `json:"licenseDeclared,omitempty"`
	CopyrightText    string `json:"copyrightText"`
	Comment          string `json:"comment,omitempty"`
}

type spdxRel struct {
	Element      string `json:"spdxElementId"`
	Relationship string `json:"relationshipType"`
	Related      string `json:"relatedSpdxElement"`
}

// SPDXJSON writes an SPDX 2.3 document derived from the canonical BOM.
func SPDXJSON(w io.Writer, b *bom.BOM) error {
	root := spdxPackage{
		Name:             b.Repository.Path,
		SPDXID:           "SPDXRef-Repository",
		DownloadLocation: "NOASSERTION",
		FilesAnalyzed:    false,
		LicenseConcluded: "NOASSERTION",
		CopyrightText:    "NOASSERTION",
		Comment:          "Infrastructure-as-Code repository",
	}

	doc := spdxDoc{
		SPDXVersion:  "SPDX-2.3",
		DataLicense:  "CC0-1.0",
		SPDXID:       "SPDXRef-DOCUMENT",
		Name:         "iacbom-" + sanitizeID(b.Repository.Path),
		CreationInfo: spdxCreation{Created: spdxCreated, Creators: []string{"Tool: iacbom-" + b.Generator.Version}},
		Packages:     []spdxPackage{root},
	}
	doc.DocumentNamespace = fmt.Sprintf("https://iacbom.dev/spdx/%s", contentHash(b))

	depsOn := func(id string) {
		doc.Relationships = append(doc.Relationships, spdxRel{
			Element: root.SPDXID, Relationship: "DEPENDS_ON", Related: id,
		})
	}

	addPkg := func(name, version, kindComment string) string {
		id := "SPDXRef-Package-" + sanitizeID(name)
		if version != "" && version != "unknown" {
			id += "-" + sanitizeID(version)
		}
		for _, p := range doc.Packages {
			if p.SPDXID == id {
				return id
			}
		}
		doc.Packages = append(doc.Packages, spdxPackage{
			Name:             name,
			SPDXID:           id,
			VersionInfo:      version,
			DownloadLocation: "NOASSERTION",
			FilesAnalyzed:    false,
			LicenseConcluded: "NOASSERTION",
			CopyrightText:    "NOASSERTION",
			Comment:          kindComment,
		})
		return id
	}

	for _, rt := range b.Runtimes {
		depsOn(addPkg(rt.Name, rt.Version, "iacbom: IaC runtime"))
	}
	for _, p := range b.Providers {
		depsOn(addPkg(p.Source, firstNonEmptyStr(p.Locked, ""), "iacbom: terraform provider"))
	}
	for _, m := range b.Modules {
		name := m.Source
		version := firstNonEmptyStr(m.Version, m.Ref)
		if m.Kind == bom.ModuleLocal {
			name = fmt.Sprintf("%s (%s)", m.Source, m.Name)
		}
		depsOn(addPkg(name, version, "iacbom: terraform module ("+string(m.Kind)+")"))
	}
	for _, t := range b.Tools {
		depsOn(addPkg(t.Name, t.Version, "iacbom: tool ("+t.Category+")"))
	}

	return writeJSON(w, doc)
}

func contentHash(b *bom.BOM) string {
	j, _ := json.Marshal(b)
	sum := sha256.Sum256(j)
	return hex.EncodeToString(sum[:8])
}

func sanitizeID(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-':
			out = append(out, r)
		default:
			out = append(out, '-')
		}
	}
	return string(out)
}
