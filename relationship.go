package gopc

import (
	"crypto/rand"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// TargetMode is an enumerable for the different target modes.
type TargetMode int

const (
	// ModeInternal when the target targetMode is Internal (default value).
	// Target points to a part within the package and target uri must be relative.
	ModeInternal TargetMode = iota
	// ModeExternal when the target targetMode is External.
	// Target points to an external resource and target uri can be relative or absolute.
	ModeExternal
)

const externalMode = "External"

const (
	// RelTypeMetaDataCoreProps defines a core properties relationship.
	RelTypeMetaDataCoreProps = "http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties"
	// RelTypeDigitalSignature defines a digital signature relationship.
	RelTypeDigitalSignature = "http://schemas.openxmlformats.org/package/2006/relationships/digital-signature/signature"
	// RelTypeDigitalSignatureOrigin defines a digital signature origin relationship.
	RelTypeDigitalSignatureOrigin = "http://schemas.openxmlformats.org/package/2006/relationships/digital-signature/origin"
	// RelTypeDigitalSignatureCert defines a digital signature certificate relationship.
	RelTypeDigitalSignatureCert = "http://schemas.openxmlformats.org/package/2006/relationships/digital-signature/certificate"
	// RelTypeThumbnail defines a thumbnail relationship.
	RelTypeThumbnail = "http://schemas.openxmlformats.org/package/2006/relationships/metadata/thumbnail"
)

// Relationship is used to express a relationship between a source and a target part.
// The only way to create a Relationship, is to call the Part.CreateRelationship()
// or Package.CreateRelationship(). A relationship is owned by a part or by the package itself.
// If the source part is deleted all the relationships it owns are also deleted.
// A target of the relationship need not be present.
// Defined in ISO/IEC 29500-2 §9.3.
type Relationship struct {
	ID         string
	RelType    string
	TargetURI  string
	TargetMode TargetMode
	sourceURI  string
}

type relationshipsXML struct {
	XMLName xml.Name           `xml:"Relationships"`
	XML     string             `xml:"xmlns,attr"`
	RelsXML []*relationshipXML `xml:"Relationship"`
}

type relationshipXML struct {
	ID        string `xml:"Id,attr"`
	RelType   string `xml:"Type,attr"`
	TargetURI string `xml:"Target,attr"`
	Mode      string `xml:"TargetMode,attr,omitempty"`
}

func (r *Relationship) validate() error {
	// ISO/IEC 29500-2 M1.26
	if strings.TrimSpace(r.ID) == "" {
		return errors.New("OPC: relationship identifier cannot be empty string or a string with just spaces")
	}
	if strings.TrimSpace(r.RelType) == "" {
		return errors.New("OPC: relationship type cannot be empty string or a string with just spaces")
	}
	return validateRelationshipTarget(r.sourceURI, r.TargetURI, r.TargetMode)
}

func (r *Relationship) toXML() *relationshipXML {
	var targetMode string
	if r.TargetMode == ModeExternal {
		targetMode = externalMode
	}
	x := &relationshipXML{ID: r.ID, RelType: r.RelType, TargetURI: r.TargetURI, Mode: targetMode}
	if r.TargetMode == ModeInternal {
		if !strings.HasPrefix(x.TargetURI, "/") && !strings.HasPrefix(x.TargetURI, "\\") && !strings.HasPrefix(x.TargetURI, ".") {
			x.TargetURI = "/" + x.TargetURI
		}
	}
	return x
}

// isRelationshipURI returns true if the uri points to a relationship part.
func isRelationshipURI(uri string) bool {
	up := strings.ToUpper(uri)
	if !strings.HasSuffix(up, ".RELS") {
		return false
	}

	if strings.EqualFold(up, "/_RELS/.RELS") {
		return true
	}

	if strings.EqualFold(up, "_rels/.rels") {
		return true
	}

	eq := false
	// Look for pattern that matches: "XXX/_rels/YYY.rels" where XXX is zero or more part name characters and
	// YYY is any legal part name characters
	segments := strings.Split(up, "/")
	ls := len(segments)
	if ls >= 3 && len(segments[ls-1]) > len(".RELS") {
		eq = strings.EqualFold(segments[ls-2], "_RELS")
	}
	return eq
}

// validateRelationshipTarget checks that a relationship target follows the constrains specified in the ISO/IEC 29500-2 §9.3.
func validateRelationshipTarget(sourceURI, targetURI string, targetMode TargetMode) error {
	// ISO/IEC 29500-2 M1.28
	uri, err := url.Parse(strings.TrimSpace(targetURI))
	if err != nil || uri.String() == "" {
		return errors.New("OPC: relationship target URI reference shall be a URI or a relative reference")
	}

	// ISO/IEC 29500-2 M1.29
	if targetMode == ModeInternal && uri.IsAbs() {
		return errors.New("OPC: relationship target URI must be relative if the TargetMode is Internal")
	}

	var result error
	if targetMode != ModeExternal && !uri.IsAbs() {
		source, err := url.Parse(strings.TrimSpace(sourceURI))
		if err != nil || source.String() == "" {
			// ISO/IEC 29500-2 M1.28
			result = errors.New("OPC: relationship source URI reference shall be a URI or a relative reference")
		} else if isRelationshipURI(source.ResolveReference(uri).String()) {
			// ISO/IEC 29500-2 M1.26
			result = errors.New("OPC: The relationships part shall not have relationships to any other part")
		}
	}

	return result
}

func validateRelationships(rs []*Relationship) error {
	var s struct{}
	ids := make(map[string]struct{}, 0)
	for _, r := range rs {
		if err := r.validate(); err != nil {
			return err
		}
		// ISO/IEC 29500-2 M1.26
		if _, ok := ids[r.ID]; ok {
			return errors.New("OPC: reltionship ID shall be unique within the Relationships part")
		}
		ids[r.ID] = s
	}
	return nil
}

func uniqueRelationshipID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func encodeRelationships(w io.Writer, rs []*Relationship) error {
	w.Write(([]byte)(`<?xml version="1.0" encoding="UTF-8"?>`))
	re := &relationshipsXML{XML: "http://schemas.openxmlformats.org/package/2006/relationships"}
	for _, r := range rs {
		re.RelsXML = append(re.RelsXML, r.toXML())
	}
	return xml.NewEncoder(w).Encode(re)
}

func decodeRelationships(r io.Reader) ([]*Relationship, error) {
	relDecode := new(relationshipsXML)
	if err := xml.NewDecoder(r).Decode(relDecode); err != nil {
		return nil, err
	}
	rel := make([]*Relationship, len(relDecode.RelsXML))
	for i, rl := range relDecode.RelsXML {

		// Add SourceURI --> path (?)

		rel[i] = &Relationship{ID: rl.ID, TargetURI: rl.TargetURI, RelType: rl.RelType}
		if rl.Mode == "" {
			rel[i].TargetMode = ModeInternal
		} else {
			rel[i].TargetMode = ModeExternal
		}
	}
	return rel, nil
}

type relationshipsPart struct {
	relation map[string][]*Relationship // partname:relationship
}

func (rp *relationshipsPart) findRelationship(name string) []*Relationship {
	if rp.relation == nil {
		rp.relation = make(map[string][]*Relationship)
	}
	if rel, ok := rp.relation[name]; ok {
		return rel
	}
	return nil
}

func (rp *relationshipsPart) addRelationship(name string, r []*Relationship) {
	if rp.relation == nil {
		rp.relation = make(map[string][]*Relationship)
	}
	rp.relation[name] = r
}
