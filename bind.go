// Copyright 2016 Sam Whited.
// Use of this source code is governed by the BSD 2-clause license that can be
// found in the LICENSE file.

package xmpp

import (
	"context"
	"encoding/xml"
	"io"

	"mellium.im/xmpp/internal"
	"mellium.im/xmpp/internal/ns"
	"mellium.im/xmpp/jid"
	"mellium.im/xmpp/stanza"
	"mellium.im/xmpp/stream"
)

// BindResource is a stream feature that can be used for binding a resource
// (client) to the stream.
//
// Resource binding is the final feature negotiated when setting up a new
// session and is required to allow communiation with other clients and servers
// in the network. Resource binding is mandatory-to-negotiate. Resources
// generated by the server during binding must be unguessable to prevent certain
// security issues related to guessing resourceparts.
//
// If used on a server connection, BindResource generates and assigns random
// resourceparts, however this default is subject to change.
func BindResource() StreamFeature {
	return BindCustom(nil)
}

// BindCustom is identical to BindResource when used on a client session, but
// but for server sessions the server function is called to generate the JID
// that should be returned to the client. If server is nil, BindCustom is
// identical to BindResource.
func BindCustom(server func(*jid.JID, string) (*jid.JID, error)) StreamFeature {
	return StreamFeature{
		Name:       xml.Name{Space: ns.Bind, Local: "bind"},
		Necessary:  Authn,
		Prohibited: Ready,
		List: func(ctx context.Context, e *xml.Encoder, start xml.StartElement) (req bool, err error) {
			req = true
			if err = e.EncodeToken(start); err != nil {
				return req, err
			}
			if err = e.EncodeToken(start.End()); err != nil {
				return req, err
			}

			err = e.Flush()
			return req, err
		},
		Parse: func(ctx context.Context, d *xml.Decoder, start *xml.StartElement) (bool, interface{}, error) {
			parsed := struct {
				XMLName xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-bind bind"`
			}{}
			return true, nil, d.DecodeElement(&parsed, start)
		},
		Negotiate: func(ctx context.Context, session *Session, data interface{}) (mask SessionState, rw io.ReadWriter, err error) {
			e := session.Encoder()
			d := session.Decoder()

			// Handle the server side of resource binding if we're on the receiving
			// end of the connection.
			if (session.State() & Received) == Received {
				var j *jid.JID
				var err error
				if server != nil {
					j, err = server(session.RemoteAddr(), data.(string))
				} else {
					// TODO: Add and use a method to *jid.JID to copy a JID, changing the
					// resource (and only processing the resource).
					j = session.RemoteAddr()
					j, err = jid.New(j.Localpart(), j.Domainpart(), internal.RandomID())
				}
				if err != nil {
					return mask, nil, err
				}
				bindStart := xml.StartElement{Name: xml.Name{Local: "bind", Space: ns.Bind}}
				jidStart := xml.StartElement{Name: xml.Name{Local: "jid"}}
				if err = e.EncodeToken(bindStart); err != nil {
					return mask, nil, err
				}
				if err = e.EncodeToken(jidStart); err != nil {
					return mask, nil, err
				}
				if err = e.EncodeToken(xml.CharData(j.String())); err != nil {
					return mask, nil, err
				}
				if err = e.EncodeToken(jidStart.End()); err != nil {
					return mask, nil, err
				}
				if err = e.EncodeToken(bindStart.End()); err != nil {
					return mask, nil, err
				}
				return mask, nil, e.Flush()
			}

			// Client encodes an IQ requesting resource binding.
			reqID := internal.RandomID()
			err = e.Encode(struct {
				stanza.IQ

				Bind struct {
					Resource string `xml:"resource,omitempty"`
				} `xml:"urn:ietf:params:xml:ns:xmpp-bind bind"`
			}{
				IQ: stanza.IQ{
					ID:   reqID,
					Type: stanza.SetIQ,
				},
				Bind: struct {
					Resource string `xml:"resource,omitempty"`
				}{
					Resource: session.Config().Origin.Resourcepart(),
				},
			})
			if err != nil {
				return mask, nil, err
			}

			// Client waits on an IQ response.
			//
			// We duplicate a lot of what should be stream-level IQ logic here; that
			// could maybe be fixed in the future, but it's necessary right now
			// because being able to use an IQ at all during resource negotiation is a
			// special case in XMPP that really shouldn't be valid (and is fixed in
			// current working drafts for a bind replacement).
			tok, err := d.Token()
			if err != nil {
				return mask, nil, err
			}
			start, ok := tok.(xml.StartElement)
			if !ok {
				return mask, nil, stream.BadFormat
			}
			resp := struct {
				stanza.IQ
				Bind struct {
					JID *jid.JID `xml:"jid"`
				} `xml:"urn:ietf:params:xml:ns:xmpp-bind bind"`
				Err stanza.Error `xml:"error"`
			}{}
			switch start.Name {
			case xml.Name{Space: ns.Client, Local: "iq"}:
				if err = d.DecodeElement(&resp, &start); err != nil {
					return mask, nil, err
				}
			default:
				return mask, nil, stream.BadFormat
			}

			switch {
			case resp.ID != reqID:
				return mask, nil, stream.UndefinedCondition
			case resp.Type == stanza.ResultIQ:
				session.origin = resp.Bind.JID
			case resp.Type == stanza.ErrorIQ:
				return mask, nil, resp.Err
			default:
				return mask, nil, stanza.Error{Condition: stanza.BadRequest}
			}
			return Ready, nil, nil
		},
	}
}
