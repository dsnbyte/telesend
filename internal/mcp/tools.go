package mcp

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/dsnbyte/telesend/internal/app"
	"github.com/dsnbyte/telesend/internal/apperr"
	"github.com/dsnbyte/telesend/internal/store"
	"github.com/dsnbyte/telesend/internal/version"
)

// Authorizer checks and canonicalizes media sources in an MCP payload.
type Authorizer interface {
	Authorize(payload any) (any, error)
}

// LocalAuthorizer canonicalizes paths using FilePolicy.
type LocalAuthorizer struct{ Policy *FilePolicy }

func isChatGPTFileID(id string) bool {
	return strings.HasPrefix(id, "file_") || strings.HasPrefix(id, "file-")
}

// Authorize resolves path media sources through the policy.
func (a *LocalAuthorizer) Authorize(v any) (any, error) {
	switch t := v.(type) {
	case map[string]any:
		src, ok := t["source"].(string)
		if !ok {
			if durl, okDurl := t["download_url"].(string); okDurl && durl != "" {
				src = "url"
				ok = true
			} else if u, okU := t["url"].(string); okU && u != "" {
				src = "url"
				ok = true
			} else if fid, okFid := t["file_id"].(string); okFid && fid != "" {
				if !isChatGPTFileID(fid) {
					src = "file_id"
					ok = true
				}
			} else if val, okVal := t["value"].(string); okVal && val != "" {
				if strings.HasPrefix(val, "http://") || strings.HasPrefix(val, "https://") {
					src = "url"
					ok = true
				} else if strings.HasPrefix(val, "/") || strings.HasPrefix(val, "./") {
					src = "path"
					ok = true
				} else if !isChatGPTFileID(val) {
					src = "file_id"
					ok = true
				}
			}
		}
		if durl, okDurl := t["download_url"].(string); okDurl && durl != "" {
			src = "url"
			ok = true
		}
		if ok && (src == "path" || src == "url" || src == "file_id") {
			val, _ := t["value"].(string)
			if durl, okDurl := t["download_url"].(string); okDurl && durl != "" {
				val = durl
			} else if u, okU := t["url"].(string); okU && u != "" {
				val = u
			} else if val == "" {
				if fid, okFid := t["file_id"].(string); okFid && fid != "" {
					val = fid
				}
			}
			if val == "" {
				return nil, apperr.Newf(apperr.Validation, "MCP media source %s requires a value", src)
			}
			if isChatGPTFileID(val) && t["download_url"] == nil {
				return nil, apperr.New(apperr.Validation,
					"ChatGPT file ID must be passed via the top-level 'file' parameter (e.g. send_media(to=..., file=...)) so ChatGPT can supply the file data")
			}
			if src == "path" {
				canon, err := a.Policy.Authorize(val)
				if err != nil {
					return nil, err
				}
				out := maps.Clone(t)
				out["source"] = src
				out["value"] = canon
				if fn, okFn := t["file_name"].(string); okFn && fn != "" {
					if _, hasFn := out["filename"]; !hasFn {
						out["filename"] = fn
					}
				}
				return out, nil
			}
			out := maps.Clone(t)
			out["source"] = src
			out["value"] = val
			if fn, okFn := t["file_name"].(string); okFn && fn != "" {
				if _, hasFn := out["filename"]; !hasFn {
					out["filename"] = fn
				}
			}
			return out, nil
		}
		if ok && src == "upload" {
			return nil, apperr.New(apperr.Validation, "MCP media source must be path, url, or file_id")
		}
		out := make(map[string]any, len(t))
		for k, val := range t {
			sub, err := a.Authorize(val)
			if err != nil {
				return nil, err
			}
			out[k] = sub
		}
		return out, nil
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			sub, err := a.Authorize(val)
			if err != nil {
				return nil, err
			}
			out[i] = sub
		}
		return out, nil
	}
	return v, nil
}

// RemoteAuthorizer rejects path sources and only permits url and file_id.
type RemoteAuthorizer struct{}

// Authorize rejects server filesystem paths.
func (RemoteAuthorizer) Authorize(v any) (any, error) {
	switch t := v.(type) {
	case map[string]any:
		src, ok := t["source"].(string)
		if !ok {
			if durl, okDurl := t["download_url"].(string); okDurl && durl != "" {
				src = "url"
				ok = true
			} else if u, okU := t["url"].(string); okU && u != "" {
				src = "url"
				ok = true
			} else if fid, okFid := t["file_id"].(string); okFid && fid != "" {
				if !isChatGPTFileID(fid) {
					src = "file_id"
					ok = true
				}
			} else if val, okVal := t["value"].(string); okVal && val != "" {
				if strings.HasPrefix(val, "http://") || strings.HasPrefix(val, "https://") {
					src = "url"
					ok = true
				} else if !strings.HasPrefix(val, "/") && !strings.HasPrefix(val, "./") && !isChatGPTFileID(val) {
					src = "file_id"
					ok = true
				}
			}
		}
		if durl, okDurl := t["download_url"].(string); okDurl && durl != "" {
			src = "url"
			ok = true
		}
		if ok && (src == "path" || src == "url" || src == "file_id" || src == "upload") {
			if src == "path" {
				return nil, apperr.New(apperr.FilesystemPolicy,
					"Remote MCP cannot use server filesystem paths; use a public HTTPS URL or Telegram file_id")
			}
			if src != "url" && src != "file_id" {
				return nil, apperr.New(apperr.Validation,
					"Remote MCP media must use a media URL or Telegram file_id")
			}
			val, _ := t["value"].(string)
			if durl, okDurl := t["download_url"].(string); okDurl && durl != "" {
				val = durl
			} else if u, okU := t["url"].(string); okU && u != "" {
				val = u
			} else if val == "" {
				if fid, okFid := t["file_id"].(string); okFid && fid != "" {
					val = fid
				}
			}
			if val == "" {
				return nil, apperr.Newf(apperr.Validation, "MCP media source %s requires a value", src)
			}
			if isChatGPTFileID(val) && t["download_url"] == nil {
				return nil, apperr.New(apperr.Validation,
					"ChatGPT file ID must be passed via the top-level 'file' parameter (e.g. send_media(to=..., file=...)) so ChatGPT can supply the file data")
			}
			out := maps.Clone(t)
			out["source"] = src
			out["value"] = val
			if fn, okFn := t["file_name"].(string); okFn && fn != "" {
				if _, hasFn := out["filename"]; !hasFn {
					out["filename"] = fn
				}
			}
			return out, nil
		}
		out := make(map[string]any, len(t))
		for k, val := range t {
			sub, err := RemoteAuthorizer{}.Authorize(val)
			if err != nil {
				return nil, err
			}
			out[k] = sub
		}
		return out, nil
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			sub, err := RemoteAuthorizer{}.Authorize(val)
			if err != nil {
				return nil, err
			}
			out[i] = sub
		}
		return out, nil
	}
	return v, nil
}

// ToolOptions configures tool creation.
type ToolOptions struct {
	CanRead    bool
	CanSend    bool
	Authorizer Authorizer
	IsRemote   bool
}

func envelopeSchema(payloadProps M) M {
	payloadProps["disable_notification"] = boolean("Send silently without notifying the recipient.")
	payloadSchema := M{
		"type":                 "object",
		"properties":           payloadProps,
		"additionalProperties": true,
	}
	topProps := M{
		"to":              str("Chat ID, @username, or alias from list_aliases"),
		"bot":             str("Bot username; default from list_bots if omitted"),
		"messageThreadId": posInt("Optional message thread ID (forum topic)"),
		"payload":         payloadSchema,
	}
	for k, v := range payloadProps {
		if _, exists := topProps[k]; !exists {
			topProps[k] = v
		}
	}
	return M{
		"type":                 "object",
		"properties":           topProps,
		"required":             []string{"to"},
		"additionalProperties": true,
	}
}

func mediaSourceSchema(allowURL, allowPath bool) M {
	sources := []string{}
	if allowPath {
		sources = append(sources, "path")
	}
	if allowURL {
		sources = append(sources, "url")
	}
	sources = append(sources, "file_id")

	desc := "Use a policy-approved path, a public URL, ChatGPT-generated file, or a Telegram file_id."
	if !allowPath {
		if allowURL {
			desc = "Use a direct media URL, ChatGPT-generated file, or a Telegram file_id."
		} else {
			desc = "Use a Telegram file_id."
		}
	}
	return M{
		"type": "object",
		"properties": M{
			"source":       enum(desc, sources...),
			"value":        strAny(desc),
			"download_url": strAny("Temporary download URL used by ChatGPT file handoff."),
			"file_id":      strAny("Telegram file_id or ChatGPT file ID."),
			"mime_type":    strAny("MIME type of the media file."),
			"file_name":    strAny("Filename of the media file."),
			"filename":     strAny("Optional filename for the media attachment."),
		},
		"additionalProperties": true,
	}
}

func parseEnvelope(args map[string]any) (to, bot string, thread int, payload map[string]any, err error) {
	to, _ = args["to"].(string)
	if to == "" {
		return "", "", 0, nil, apperr.New(apperr.Validation, "Field \"to\" is required")
	}
	bot, _ = args["bot"].(string)
	if t, ok := args["messageThreadId"].(int); ok {
		thread = t
	} else if f, ok := asFloat(args["messageThreadId"]); ok {
		thread = int(f)
	}
	p, ok := args["payload"].(map[string]any)
	if ok && p != nil {
		payload = maps.Clone(p)
	} else {
		payload = make(map[string]any)
	}
	for k, v := range args {
		if k != "to" && k != "bot" && k != "messageThreadId" && k != "payload" {
			if _, exists := payload[k]; !exists {
				payload[k] = v
			}
		}
	}
	return to, bot, thread, payload, nil
}

// BuildTools constructs the 11 MCP tools configured with opts.
func BuildTools(a *app.App, opts ToolOptions) []Tool {
	allowPath := !opts.IsRemote
	tools := []Tool{
		{
			Name:        "list_aliases",
			Title:       "List aliases",
			Description: "List Telesend recipient aliases for Telegram chats that can be used as `to`. Each alias includes type (`group` or `private`). Optional filter (`group` or `private`) allows listing only group or private aliases.",
			Schema: M{
				"type": "object",
				"properties": M{
					"filter":  enum("Filter aliases by type: \"group\" or \"private\".", "group", "private"),
					"type":    enum("Filter aliases by type: \"group\" or \"private\".", "group", "private"),
					"group":   boolean("Filter to only group aliases."),
					"private": boolean("Filter to only private aliases."),
				},
				"additionalProperties": true,
			},
			ReadOnly: true,
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				if !opts.CanRead {
					return nil, apperr.New(apperr.Unauthorized, "MCP scope mcp:read is required")
				}
				var filter string
				if t, ok := args["type"].(string); ok && t != "" {
					filter = t
				}
				if f, ok := args["filter"].(string); ok && f != "" {
					if filter != "" && filter != f {
						return nil, apperr.New(apperr.Validation, "Conflicting filter arguments")
					}
					filter = f
				}
				g, _ := args["group"].(bool)
				p, _ := args["private"].(bool)
				if g && p {
					return nil, apperr.New(apperr.Validation, "Cannot filter by both group and private")
				}
				if g {
					if filter != "" && filter != store.TypeGroup {
						return nil, apperr.New(apperr.Validation, "Conflicting filter arguments")
					}
					filter = store.TypeGroup
				}
				if p {
					if filter != "" && filter != store.TypePrivate {
						return nil, apperr.New(apperr.Validation, "Conflicting filter arguments")
					}
					filter = store.TypePrivate
				}
				aliases, err := a.Aliases.List(filter)
				if err != nil {
					return nil, err
				}
				if aliases == nil {
					aliases = []store.Alias{}
				}
				return map[string]any{"aliases": aliases}, nil
			},
		},
		{
			Name:        "list_bots",
			Title:       "List bots",
			Description: "List Telegram bots registered in Telesend and which one is the default.",
			Schema:      M{"type": "object", "properties": M{}, "additionalProperties": true},
			ReadOnly:    true,
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				if !opts.CanRead {
					return nil, apperr.New(apperr.Unauthorized, "MCP scope mcp:read is required")
				}
				bots, err := a.Bots.List()
				if err != nil {
					return nil, err
				}
				if bots == nil {
					bots = []store.Bot{}
				}
				return map[string]any{"bots": bots}, nil
			},
		},
		{
			Name:  "get_telegram_parameter_doc",
			Title: "Get Telegram parameter docs",
			Description: "Return advanced parameter docs for a Telegram Bot API method or Telesend message type. " +
				"ONLY call this tool if you need advanced parameters or customization options not available in the send_* tool schemas. " +
				"DO NOT call this tool if the basic parameters (e.g. text, parse_mode, caption, file, type, disable_notification) are sufficient for your task. " +
				"Accepts Telegram method names (e.g. \"sendMessage\", \"sendPhoto\") or Telesend types (e.g. \"text\", \"photo\", \"poll\").",
			Schema: M{
				"type": "object",
				"properties": M{
					"method": str("Telegram method name or Telesend type. Known values: " + strings.Join(KnownQueries, ", ")),
				},
				"required":             []string{"method"},
				"additionalProperties": true,
			},
			ReadOnly: true,
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				method, _ := args["method"].(string)
				doc, ok := MethodDocFor(method)
				if !ok {
					return nil, apperr.Newf(apperr.NotFound, "No docs found for %q. Valid queries: %s", method, strings.Join(KnownQueries, ", "))
				}
				return doc, nil
			},
		},
		{
			Name:  "send_text",
			Title: "Send text message",
			Description: "Send a Telegram text message via Telesend. Use parse_mode: \"HTML\" or \"MarkdownV2\" for formatting. " +
				"Add draft_id to send as a streaming draft instead of a regular message.",
			Schema: envelopeSchema(M{
				"text":       strAny("Message text. Required unless draft_id is set."),
				"draft_id":   posInt("Set to send as a streaming draft (thinking indicator) instead of a chat message."),
				"parse_mode": enum("Text formatting mode. Recommended: \"HTML\" or \"MarkdownV2\".", "HTML", "MarkdownV2", "Markdown"),
			}),
			ReadOnly: false,
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				if !opts.CanSend {
					return nil, apperr.New(apperr.Unauthorized, "MCP scope mcp:send is required")
				}
				to, bot, thread, payload, err := parseEnvelope(args)
				if err != nil {
					return nil, err
				}
				draftID, isDraft := payload["draft_id"]
				text, _ := payload["text"].(string)
				if !isDraft && text == "" {
					return nil, apperr.New(apperr.Validation, "Field \"text\" is required when draft_id is not set")
				}
				typ := "text"
				if isDraft {
					typ = "message-draft"
					payload["draft_id"] = draftID
				}
				authorized, err := opts.Authorizer.Authorize(payload)
				if err != nil {
					return nil, err
				}
				return a.Send(ctx, app.SendRequest{
					Type: typ, To: to, Bot: bot, ThreadID: thread, Payload: authorized.(map[string]any),
				})
			},
		},
		{
			Name:  "send_rich_message",
			Title: "Send rich message",
			Description: "Send a rich/structured Telegram message via Telesend. " +
				"Add draft_id to send as a streaming draft instead.",
			Schema: envelopeSchema(M{
				"rich_message": looseObj("Rich message content, e.g. { markdown: '**Hello**' }."),
				"draft_id":     posInt("Set to send as a streaming rich-message draft."),
			}),
			ReadOnly: false,
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				if !opts.CanSend {
					return nil, apperr.New(apperr.Unauthorized, "MCP scope mcp:send is required")
				}
				to, bot, thread, payload, err := parseEnvelope(args)
				if err != nil {
					return nil, err
				}
				typ := "rich-message"
				if _, isDraft := payload["draft_id"]; isDraft {
					typ = "rich-message-draft"
				}
				authorized, err := opts.Authorizer.Authorize(payload)
				if err != nil {
					return nil, err
				}
				return a.Send(ctx, app.SendRequest{
					Type: typ, To: to, Bot: bot, ThreadID: thread, Payload: authorized.(map[string]any),
				})
			},
		},
		{
			Name:  "send_media",
			Title: "Send media file",
			Description: "Send a single Telegram media file via Telesend (supports ChatGPT-generated files). Set type to: photo, video, animation, audio, document, sticker, voice, video_note, or live_photo. " +
				"video_note and live_photo do not support URL sources. " +
				"live_photo also requires a photo field (the still frame). " +
				"Add caption for a text caption below the media.",
			Meta: map[string]any{
				"openai/fileParams": []string{"file"},
			},
			Schema: envelopeSchema(M{
				"type":       enum("Media type to send.", "photo", "video", "animation", "audio", "document", "sticker", "voice", "video_note", "live_photo"),
				"file":       mediaSourceSchema(true, allowPath),
				"photo":      mediaSourceSchema(false, allowPath),
				"caption":    strAny("Optional caption displayed below the media (max 1024 chars)."),
				"parse_mode": enum("Parse mode for the caption.", "HTML", "MarkdownV2", "Markdown"),
			}),
			ReadOnly: false,
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				if !opts.CanSend {
					return nil, apperr.New(apperr.Unauthorized, "MCP scope mcp:send is required")
				}
				to, bot, thread, payload, err := parseEnvelope(args)
				if err != nil {
					return nil, err
				}
				mediaType, _ := payload["type"].(string)
				if mediaType == "" {
					mediaType = "photo"
				}
				validTypes := []string{"photo", "video", "animation", "audio", "document", "sticker", "voice", "video_note", "live_photo"}
				if !slices.Contains(validTypes, mediaType) {
					return nil, apperr.Newf(apperr.Validation, "Invalid media type %q. Use: %s", mediaType, strings.Join(validTypes, ", "))
				}
				if s, ok := payload["file"].(string); ok && s != "" {
					payload["file"] = map[string]any{"value": s}
				}
				if s, ok := payload["photo"].(string); ok && s != "" {
					payload["photo"] = map[string]any{"value": s}
				}
				file, hasFile := payload["file"].(map[string]any)
				if !hasFile || file == nil {
					file, hasFile = payload["photo"].(map[string]any)
				}
				if !hasFile || file == nil {
					return nil, apperr.New(apperr.Validation, "Field \"file\" is required for send_media")
				}
				if file != nil {
					if durl, ok := file["download_url"].(string); ok && durl != "" {
						file["value"] = durl
					} else if u, ok := file["url"].(string); ok && u != "" {
						file["value"] = u
					} else if v, _ := file["value"].(string); v == "" {
						if fid, ok := file["file_id"].(string); ok && fid != "" {
							file["value"] = fid
						}
					}
					if fn, ok := file["file_name"].(string); ok && fn != "" {
						if _, has := file["filename"]; !has {
							file["filename"] = fn
						}
					}
				}
				if (mediaType == "video_note" || mediaType == "live_photo") && file["source"] == "url" {
					return nil, apperr.Newf(apperr.Validation, "URL source is not supported for type %q", mediaType)
				}
				catalogType := mediaType
				if mediaType == "video_note" {
					catalogType = "video-note"
				} else if mediaType == "live_photo" {
					catalogType = "live-photo"
				}
				fieldName := mediaType
				delete(payload, "type")
				delete(payload, "file")
				payload[fieldName] = file

				authorized, err := opts.Authorizer.Authorize(payload)
				if err != nil {
					return nil, err
				}
				return a.Send(ctx, app.SendRequest{
					Type: catalogType, To: to, Bot: bot, ThreadID: thread, Payload: authorized.(map[string]any),
				})
			},
		},
		{
			Name:  "send_media_group",
			Title: "Send media album",
			Description: "Send 2–10 Telegram media files as an album via Telesend (supports ChatGPT-generated files). " +
				"Add star_count to send as paid media locked behind Telegram Stars. " +
				"Paid media does not support URL sources.",
			Meta: map[string]any{
				"openai/fileParams": []string{"media"},
			},
			Schema: envelopeSchema(M{
				"media": arr(M{
					"type": "object",
					"properties": M{
						"type":         strAny("Item type: photo, video, audio, or document (defaults to photo)."),
						"media":        mediaSourceSchema(true, allowPath),
						"download_url": strAny("Temporary download URL used by ChatGPT/AI file handoff."),
						"file_id":      strAny("Telegram file_id or ChatGPT/AI file ID."),
						"mime_type":    strAny("MIME type of the media file."),
						"file_name":    strAny("Filename of the media file."),
						"caption":      strAny("Optional caption for this item."),
					},
					"additionalProperties": true,
				}, "Array of 2–10 media items."),
				"star_count": posInt("Set to send as paid media (Telegram Stars required to view). Disables URL sources."),
			}),
			ReadOnly: false,
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				if !opts.CanSend {
					return nil, apperr.New(apperr.Unauthorized, "MCP scope mcp:send is required")
				}
				to, bot, thread, payload, err := parseEnvelope(args)
				if err != nil {
					return nil, err
				}
				if rawMedia, ok := payload["media"].([]any); ok {
					normalized := make([]any, 0, len(rawMedia))
					for _, item := range rawMedia {
						if mObj, ok := item.(map[string]any); ok {
							if _, hasDurl := mObj["download_url"]; hasDurl {
								if _, hasMedia := mObj["media"]; !hasMedia {
									itemType, _ := mObj["type"].(string)
									if itemType == "" {
										itemType = "photo"
									}
									mObj = map[string]any{
										"type":  itemType,
										"media": maps.Clone(mObj),
									}
								}
							}
							normalized = append(normalized, mObj)
						} else {
							normalized = append(normalized, item)
						}
					}
					payload["media"] = normalized
				}
				typ := "media-group"
				if _, hasStars := payload["star_count"]; hasStars {
					typ = "paid-media"
				}
				authorized, err := opts.Authorizer.Authorize(payload)
				if err != nil {
					return nil, err
				}
				return a.Send(ctx, app.SendRequest{
					Type: typ, To: to, Bot: bot, ThreadID: thread, Payload: authorized.(map[string]any),
				})
			},
		},
		{
			Name:        "send_location",
			Title:       "Send location or venue",
			Description: "Send a Telegram geographic location or venue via Telesend. Add both title and address to send as a named venue instead.",
			Schema: envelopeSchema(M{
				"latitude":  number("Latitude of the location."),
				"longitude": number("Longitude of the location."),
				"title":     strAny("Venue name. Required together with address to send a venue."),
				"address":   strAny("Venue address. Required together with title to send a venue."),
			}),
			ReadOnly: false,
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				if !opts.CanSend {
					return nil, apperr.New(apperr.Unauthorized, "MCP scope mcp:send is required")
				}
				to, bot, thread, payload, err := parseEnvelope(args)
				if err != nil {
					return nil, err
				}
				title, hasTitle := payload["title"].(string)
				addr, hasAddr := payload["address"].(string)
				hasTitle = hasTitle && title != ""
				hasAddr = hasAddr && addr != ""
				if hasTitle != hasAddr {
					return nil, apperr.New(apperr.Validation, "title and address must both be set to send a venue")
				}
				typ := "location"
				if hasTitle && hasAddr {
					typ = "venue"
				}
				authorized, err := opts.Authorizer.Authorize(payload)
				if err != nil {
					return nil, err
				}
				return a.Send(ctx, app.SendRequest{
					Type: typ, To: to, Bot: bot, ThreadID: thread, Payload: authorized.(map[string]any),
				})
			},
		},
		{
			Name:        "send_contact",
			Title:       "Send contact",
			Description: "Send a Telegram phone contact card via Telesend.",
			Schema: envelopeSchema(M{
				"phone_number": str("Contact's phone number."),
				"first_name":   str("Contact's first name."),
			}),
			ReadOnly: false,
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				if !opts.CanSend {
					return nil, apperr.New(apperr.Unauthorized, "MCP scope mcp:send is required")
				}
				to, bot, thread, payload, err := parseEnvelope(args)
				if err != nil {
					return nil, err
				}
				authorized, err := opts.Authorizer.Authorize(payload)
				if err != nil {
					return nil, err
				}
				return a.Send(ctx, app.SendRequest{
					Type: "contact", To: to, Bot: bot, ThreadID: thread, Payload: authorized.(map[string]any),
				})
			},
		},
		{
			Name:  "send_interactive",
			Title: "Send interactive content",
			Description: "Send interactive Telegram content via Telesend. Set type to: poll, checklist, dice, or game. " +
				"poll: requires question and options[]. " +
				"checklist: requires business_connection_id and checklist object. " +
				"dice: no required fields (optionally set emoji: 🎲🎯🏀⚽🎳🎰). " +
				"game: requires game_short_name.",
			Schema: envelopeSchema(M{
				"type":                   enum("Interactive content type.", "poll", "checklist", "dice", "game"),
				"question":               strAny("Poll question (required for type: poll)."),
				"options":                arr(looseObj("Poll option, shape { text: string }"), "Poll answer options, 1–12 items (required for type: poll)."),
				"business_connection_id": strAny("Business connection ID (required for type: checklist)."),
				"checklist":              looseObj("Checklist object (required for type: checklist)."),
				"game_short_name":        strAny("Short name of the HTML5 game from @BotFather (required for type: game)."),
			}),
			ReadOnly: false,
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				if !opts.CanSend {
					return nil, apperr.New(apperr.Unauthorized, "MCP scope mcp:send is required")
				}
				to, bot, thread, payload, err := parseEnvelope(args)
				if err != nil {
					return nil, err
				}
				typ, _ := payload["type"].(string)
				delete(payload, "type")
				switch typ {
				case "poll":
					if q, _ := payload["question"].(string); q == "" {
						return nil, apperr.New(apperr.Validation, "Field \"question\" is required for type: poll")
					}
					if opts, _ := payload["options"].([]any); len(opts) == 0 {
						return nil, apperr.New(apperr.Validation, "Field \"options\" is required for type: poll")
					}
				case "checklist":
					if b, _ := payload["business_connection_id"].(string); b == "" {
						return nil, apperr.New(apperr.Validation, "Field \"business_connection_id\" is required for type: checklist")
					}
					if c, _ := payload["checklist"].(map[string]any); c == nil {
						return nil, apperr.New(apperr.Validation, "Field \"checklist\" is required for type: checklist")
					}
				case "game":
					if g, _ := payload["game_short_name"].(string); g == "" {
						return nil, apperr.New(apperr.Validation, "Field \"game_short_name\" is required for type: game")
					}
				case "dice":
					// no required fields
				default:
					return nil, apperr.New(apperr.Validation, "Field \"type\" is required for send_interactive")
				}
				authorized, err := opts.Authorizer.Authorize(payload)
				if err != nil {
					return nil, err
				}
				return a.Send(ctx, app.SendRequest{
					Type: typ, To: to, Bot: bot, ThreadID: thread, Payload: authorized.(map[string]any),
				})
			},
		},
		{
			Name:        "send_invoice",
			Title:       "Send invoice",
			Description: "Send a Telegram payment invoice via Telesend. Use currency: XTR for Telegram Stars payments (no provider_token needed).",
			Schema: envelopeSchema(M{
				"title":       str("Product name (1–32 chars)."),
				"description": str("Product description (1–255 chars)."),
				"payload":     str("Internal bot payload for this invoice (1–128 bytes, not shown to users)."),
				"currency":    str("Three-letter ISO 4217 currency code, or \"XTR\" for Telegram Stars."),
				"prices": arr(objectOf(M{
					"label":  str("Price label"),
					"amount": integer("Price amount in smallest units"),
				}, "label", "amount"), "Price breakdown."),
			}),
			ReadOnly: false,
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				if !opts.CanSend {
					return nil, apperr.New(apperr.Unauthorized, "MCP scope mcp:send is required")
				}
				to, bot, thread, payload, err := parseEnvelope(args)
				if err != nil {
					return nil, err
				}
				authorized, err := opts.Authorizer.Authorize(payload)
				if err != nil {
					return nil, err
				}
				return a.Send(ctx, app.SendRequest{
					Type: "invoice", To: to, Bot: bot, ThreadID: thread, Payload: authorized.(map[string]any),
				})
			},
		},
	}
	return tools
}

// NewStdioServer creates an MCP server with all tools and file policy for stdio.
func NewStdioServer(a *app.App, policy *FilePolicy) *Server {
	tools := BuildTools(a, ToolOptions{
		CanRead:    true,
		CanSend:    true,
		Authorizer: &LocalAuthorizer{Policy: policy},
		IsRemote:   false,
	})
	return NewServer("telesend", version.Version, "Send Telegram bot messages through MCP.", "", tools)
}
