// Copyright 2017 The Mellium Contributors.
// Use of this source code is governed by the BSD 2-clause
// license that can be found in the LICENSE file.

// Package disco implements XEP-0030: Service Discovery.
package disco

//go:generate go run gen.go

import (
	"encoding/xml"
	"errors"

	"mellium.im/xmlstream"
	"mellium.im/xmpp/internal/ns"
)

// Namespaces used by this package.
const (
	NSInfo  = `http://jabber.org/protocol/disco#info`
	NSItems = `http://jabber.org/protocol/disco#items`
)

type identity struct {
	Category string
	Type     string
	XMLLang  string
}

// A Registry is used to register features supported by a server.
type Registry struct {
	identities map[identity]string
	features   map[string]struct{}
}

// NewRegistry creates a new feature registry with the provided identities and
// features.
// If multiple identities are specified, the name of the registry will be used
// for all of them.
func NewRegistry(options ...Option) *Registry {
	registry := &Registry{
		features: map[string]struct{}{
			NSInfo:  struct{}{},
			NSItems: struct{}{},
		},
	}
	for _, o := range options {
		o(registry)
	}
	return registry
}

// HandleXMPP handles disco info requests.
func (r *Registry) HandleXMPP(t xmlstream.TokenReadWriter, start *xml.StartElement) error {
	// TODO: Handle the IQ and IQ semantics, not the payload.
	switch {
	case r == nil:
		return NewRegistry().HandleXMPP(t, start)
	case start == nil || start.Name.Local != "query" || start.Name.Space != NSInfo:
		return errors.New("disco: bad info query payload")
	}

	if err := xmlstream.Skip(t); err != nil {
		return err
	}

	resp := xml.StartElement{
		Name: xml.Name{Space: NSInfo, Local: "query"},
	}
	if err := t.EncodeToken(resp); err != nil {
		return err
	}

	for feature := range r.features {
		start := xml.StartElement{
			Name: xml.Name{Local: "feature"},
			Attr: []xml.Attr{
				{Name: xml.Name{Local: "var"}, Value: feature},
			},
		}
		if err := t.EncodeToken(start); err != nil {
			return err
		}
		if err := t.EncodeToken(start.End()); err != nil {
			return err
		}
	}
	for ident, name := range r.identities {
		start := xml.StartElement{
			Name: xml.Name{Local: "identity"},
			Attr: []xml.Attr{
				{Name: xml.Name{Local: "category"}, Value: ident.Category},
				{Name: xml.Name{Local: "type"}, Value: ident.Type},
				{Name: xml.Name{Local: "name"}, Value: name},
				{Name: xml.Name{Space: ns.XML, Local: "lang"}, Value: ident.XMLLang},
			},
		}
		if err := t.EncodeToken(start); err != nil {
			return err
		}
		if err := t.EncodeToken(start.End()); err != nil {
			return err
		}
	}

	return t.EncodeToken(resp.End())
}
