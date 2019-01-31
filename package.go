// Package gopc implements the ISO/IEC 29500-2, also known as the "Open Packaging Convention".
//
// The Open Packaging specification describes an abstract model and physical format conventions for the use of
// XML, Unicode, ZIP, and other openly available technologies and specifications to organize the content and
// resources of a document within a package.
//
// The OPC is the foundation technology for many new file formats: .docx, .pptx, .xlsx, .3mf, .dwfx, ...
package gopc

import (
	"encoding/xml"
	"io"
	"mime"
	"path/filepath"
	"sort"
	"strings"
)

const (
	corePropsRel            = "http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties"
	corePropsContentType    = "application/vnd.openxmlformats-package.core-properties+xml"
	corePropsDefaultName    = "/props/core.xml"
	contentTypesName        = "/[Content_Types].xml"
	relationshipContentType = "application/vnd.openxmlformats-package.relationships+xml"
	packageRelName          = "/_rels/.rels"
)

type pkg struct {
	parts        map[string]*Part
	contentTypes contentTypes
}

func newPackage() *pkg {
	return &pkg{
		parts: make(map[string]*Part, 0),
	}
}

func (p *pkg) partExists(partName string) bool {
	_, ok := p.parts[strings.ToUpper(partName)]
	return ok
}

func (p *pkg) add(part *Part) error {
	if err := part.validate(); err != nil {
		return err
	}
	upperURI := strings.ToUpper(part.Name)
	if p.partExists(upperURI) {
		return newError(112, part.Name)
	}
	if p.checkPrefixCollision(upperURI) {
		return newError(111, part.Name)
	}
	p.contentTypes.add(part.Name, part.ContentType)
	p.parts[upperURI] = part
	return nil
}

func (p *pkg) deletePart(uri string) {
	delete(p.parts, strings.ToUpper(uri))
}

func (p *pkg) checkPrefixCollision(uri string) bool {
	keys := make([]string, len(p.parts)+1)
	keys[0] = uri
	i := 1
	for k := range p.parts {
		keys[i] = k
		i++
	}
	sort.Strings(keys)

	for i, k := range keys {
		if k == uri {
			if i > 0 && p.checkStringsPrefixCollision(uri, keys[i-1]) {
				return true
			}
			if i < len(keys)-1 && p.checkStringsPrefixCollision(keys[i+1], uri) {
				return true
			}
		}
	}
	return false
}

func (p *pkg) encodeContentTypes(w io.Writer) error {
	w.Write(([]byte)(`<?xml version="1.0" encoding="UTF-8"?>`))
	return xml.NewEncoder(w).Encode(p.contentTypes.toXML())
}

func (p *pkg) checkStringsPrefixCollision(s1, s2 string) bool {
	return strings.HasPrefix(s1, s2) && len(s1) > len(s2) && s1[len(s2)] == '/'
}

type contentTypesXML struct {
	XMLName xml.Name      `xml:"Types"`
	XML     string        `xml:"xmlns,attr"`
	Types   []interface{} `xml:",any"`
}

type defaultContentTypeXML struct {
	XMLName     xml.Name `xml:"Default"`
	Extension   string   `xml:"Extension,attr"`
	ContentType string   `xml:"ContentType,attr"`
}

type overrideContentTypeXML struct {
	XMLName     xml.Name `xml:"Override"`
	PartName    string   `xml:"PartName,attr"`
	ContentType string   `xml:"ContentType,attr"`
}

type contentTypes struct {
	defaults  map[string]string // extension:contenttype
	overrides map[string]string // partname:contenttype
}

func (c *contentTypes) toXML() *contentTypesXML {
	cx := &contentTypesXML{XML: "http://schemas.openxmlformats.org/package/2006/content-types"}
	if c.defaults != nil {
		for e, ct := range c.defaults {
			cx.Types = append(cx.Types, &defaultContentTypeXML{Extension: e, ContentType: ct})
		}
	}
	if c.overrides != nil {
		for pn, ct := range c.overrides {
			cx.Types = append(cx.Types, &overrideContentTypeXML{PartName: pn, ContentType: ct})
		}
	}
	return cx
}

func (c *contentTypes) ensureDefaultsMap() {
	if c.defaults == nil {
		c.defaults = make(map[string]string, 0)
	}
}

func (c *contentTypes) ensureOverridesMap() {
	if c.overrides == nil {
		c.overrides = make(map[string]string, 0)
	}
}

// Add needs a valid content type, else the behaviour is undefined
func (c *contentTypes) add(partName, contentType string) error {
	// Process descrived in ISO/IEC 29500-2 §10.1.2.3
	t, params, _ := mime.ParseMediaType(contentType)
	contentType = mime.FormatMediaType(t, params)

	ext := strings.ToLower(filepath.Ext(partName))
	if len(ext) == 0 {
		c.addOverride(partName, contentType)
		return nil
	}
	ext = ext[1:] // remove dot
	c.ensureDefaultsMap()
	currentType, ok := c.defaults[ext]
	if ok {
		if currentType != contentType {
			c.addOverride(partName, contentType)
		}
	} else {
		c.addDefault(ext, contentType)
	}

	return nil
}

func (c *contentTypes) addOverride(partName, contentType string) {
	c.ensureOverridesMap()
	// ISO/IEC 29500-2 M2.5
	c.overrides[partName] = contentType
}

func (c *contentTypes) addDefault(extension, contentType string) {
	c.ensureDefaultsMap()
	// ISO/IEC 29500-2 M2.5
	c.defaults[extension] = contentType
}

func (c *contentTypes) findType(name string) (string, error) {
	if t, ok := c.overrides[strings.ToUpper(name)]; ok {
		return t, nil
	}
	ext := filepath.Ext(name)
	if ext != "" {
		if t, ok := c.defaults[ext[1:]]; ok {
			return t, nil
		}
	}
	return "", newError(208, name)
}

type contentTypesXMLReader struct {
	XMLName xml.Name `xml:"Types"`
	XML     string   `xml:"xmlns,attr"`
	Types   []mixed  `xml:",any"`
}

type mixed struct {
	Value interface{}
}

func (m *mixed) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	switch start.Name.Local {
	case "Override":
		var e overrideContentTypeXML
		if err := d.DecodeElement(&e, &start); err != nil {
			return err
		}
		m.Value = e
	case "Default":
		var e defaultContentTypeXML
		if err := d.DecodeElement(&e, &start); err != nil {
			return err
		}
		m.Value = e
	default:
		return newError(204, "/")
	}
	return nil
}

func decodeContentTypes(r io.Reader) (*contentTypes, error) {
	ctdecode := new(contentTypesXMLReader)
	if err := xml.NewDecoder(r).Decode(ctdecode); err != nil {
		return nil, err
	}
	ct := new(contentTypes)
	for _, c := range ctdecode.Types {
		if cDefault, ok := c.Value.(defaultContentTypeXML); ok {
			ext := strings.ToLower(cDefault.Extension)
			if ext == "" {
				return nil, newError(206, "/")
			}
			if _, ok := ct.defaults[ext]; ok {
				return nil, newError(205, "/")
			}
			ct.addDefault(ext, cDefault.ContentType)
		} else if cOverride, ok := c.Value.(overrideContentTypeXML); ok {
			partName := strings.ToUpper(cOverride.PartName)
			if _, ok := ct.overrides[partName]; ok {
				return nil, newError(205, partName)
			}
			ct.addOverride(partName, cOverride.ContentType)
		}
	}
	return ct, nil
}

type corePropertiesXMLMarshal struct {
	XMLName        xml.Name `xml:"coreProperties"`
	XML            string   `xml:"xmlns,attr"`
	XMLDCTERMS     string   `xml:"xmlns:dcterms,attr"`
	XMLDC          string   `xml:"xmlns:dc,attr"`
	Category       string   `xml:"category,omitempty"`
	ContentStatus  string   `xml:"contentStatus,omitempty"`
	Created        string   `xml:"dcterms:created,omitempty"`
	Creator        string   `xml:"dc:creator,omitempty"`
	Description    string   `xml:"dc:description,omitempty"`
	Identifier     string   `xml:"dc:identifier,omitempty"`
	Keywords       string   `xml:"keywords,omitempty"`
	Language       string   `xml:"dc:language,omitempty"`
	LastModifiedBy string   `xml:"lastModifiedBy,omitempty"`
	LastPrinted    string   `xml:"lastPrinted,omitempty"`
	Modified       string   `xml:"dcterms:modified,omitempty"`
	Revision       string   `xml:"revision,omitempty"`
	Subject        string   `xml:"dc:subject,omitempty"`
	Title          string   `xml:"dc:title,omitempty"`
	Version        string   `xml:"version,omitempty"`
}

type corePropertiesXMLUnmarshal struct {
	XMLName        xml.Name `xml:"coreProperties"`
	XML            string   `xml:"xmlns,attr"`
	XMLDCTERMS     string   `xml:"dcterms,attr"`
	XMLDC          string   `xml:"dc,attr"`
	Category       string   `xml:"category,omitempty"`
	ContentStatus  string   `xml:"contentStatus,omitempty"`
	Created        string   `xml:"created,omitempty"`
	Creator        string   `xml:"creator,omitempty"`
	Description    string   `xml:"description,omitempty"`
	Identifier     string   `xml:"identifier,omitempty"`
	Keywords       string   `xml:"keywords,omitempty"`
	Language       string   `xml:"language,omitempty"`
	LastModifiedBy string   `xml:"lastModifiedBy,omitempty"`
	LastPrinted    string   `xml:"lastPrinted,omitempty"`
	Modified       string   `xml:"modified,omitempty"`
	Revision       string   `xml:"revision,omitempty"`
	Subject        string   `xml:"subject,omitempty"`
	Title          string   `xml:"title,omitempty"`
	Version        string   `xml:"version,omitempty"`
}

// CoreProperties enable users to get and set well-known and common sets of property metadata within packages.
type CoreProperties struct {
	PartName       string // Won't be writed to the package, only used to indicate the location of the CoreProperties part. If empty the default location is "/props/core.xml".
	Category       string // A categorization of the content of this package.
	ContentStatus  string // The status of the content.
	Created        string // Date of creation of the resource.
	Creator        string // An entity primarily responsible for making the content of the resource.
	Description    string // An explanation of the content of the resource.
	Identifier     string // An unambiguous reference to the resource within a given context.
	Keywords       string // A delimited set of keywords to support searching and indexing.
	Language       string // The language of the intellectual content of the resource.
	LastModifiedBy string // The user who performed the last modification.
	LastPrinted    string // The date and time of the last printing.
	Modified       string // Date on which the resource was changed.
	Revision       string // The revision number.
	Subject        string // The topic of the content of the resource.
	Title          string // The name given to the resource.
	Version        string // The version number.
}

func (c *CoreProperties) encode(w io.Writer) error {
	w.Write(([]byte)(`<?xml version="1.0" encoding="UTF-8"?>`))
	return xml.NewEncoder(w).Encode(&corePropertiesXMLMarshal{
		xml.Name{Local: "coreProperties"},
		"http://schemas.openxmlformats.org/package/2006/metadata/core-properties",
		"http://purl.org/dc/terms/",
		"http://purl.org/dc/elements/1.1/",
		c.Category, c.ContentStatus, c.Created,
		c.Creator, c.Description, c.Identifier,
		c.Keywords, c.Language, c.LastModifiedBy,
		c.LastPrinted, c.Modified, c.Revision,
		c.Subject, c.Title, c.Version,
	})
}

func decodeCoreProperties(r io.Reader) (*CoreProperties, error) {
	propDecode := new(corePropertiesXMLUnmarshal)
	if err := xml.NewDecoder(r).Decode(propDecode); err != nil {
		return nil, err
	}
	prop := &CoreProperties{Category: propDecode.Category, ContentStatus: propDecode.ContentStatus,
		Created: propDecode.Created, Creator: propDecode.Creator, Description: propDecode.Description,
		Identifier: propDecode.Identifier, Keywords: propDecode.Keywords, Language: propDecode.Language,
		LastModifiedBy: propDecode.LastModifiedBy, LastPrinted: propDecode.LastPrinted, Modified: propDecode.Modified,
		Revision: propDecode.Revision, Subject: propDecode.Subject, Title: propDecode.Title, Version: propDecode.Version}

	return prop, nil
}
