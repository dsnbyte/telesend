// Package telegram holds the Bot API client and the supported message catalog.
package telegram

// BotAPIVersion is the audited Telegram Bot API version.
const BotAPIVersion = "10.3"

// Operation describes one supported message type.
type Operation struct {
	Type        string
	Method      string
	Description string
	Required    []string
	MediaFields []string
	AllowURL    bool
	// Example is a minimal valid payload using placeholder media.
	Example map[string]any
}

// Object is shorthand for a JSON object.
type Object = map[string]any

func sampleMedia() Object { return Object{"source": "file_id", "value": "telegram-file-id"} }

func op(typ, method, desc string, required, media []string, allowURL bool, example Object) Operation {
	if example == nil {
		example = Object{}
		for _, f := range media {
			example[f] = sampleMedia()
		}
	}
	return Operation{typ, method, desc, required, media, allowURL, example}
}

func s(v ...string) []string { return v }

// Catalog lists every supported message type in a stable order.
var Catalog = []Operation{
	op("text", "sendMessage", "Text message", s("text"), nil, true, Object{"text": "Hello"}),
	op("message-draft", "sendMessageDraft", "Streaming text draft", s("draft_id"), nil, true,
		Object{"draft_id": 1, "text": "Thinking..."}),
	op("rich-message", "sendRichMessage", "Rich structured message", s("rich_message"), nil, true,
		Object{"rich_message": Object{"markdown": "**Hello**"}}),
	op("rich-message-draft", "sendRichMessageDraft", "Streaming rich message draft", s("draft_id", "rich_message"), nil, false,
		Object{"draft_id": 1, "rich_message": Object{"markdown": "**Thinking**"}}),
	op("animation", "sendAnimation", "Animation", s("animation"), s("animation"), true, nil),
	op("audio", "sendAudio", "Audio track", s("audio"), s("audio"), true, nil),
	op("document", "sendDocument", "Document", s("document"), s("document"), true, nil),
	op("live-photo", "sendLivePhoto", "Live photo", s("live_photo", "photo"), s("live_photo", "photo"), false, nil),
	op("photo", "sendPhoto", "Photo", s("photo"), s("photo"), true, nil),
	op("sticker", "sendSticker", "Sticker", s("sticker"), s("sticker"), true, nil),
	op("video", "sendVideo", "Video", s("video"), s("video"), true, nil),
	op("video-note", "sendVideoNote", "Video note", s("video_note"), s("video_note"), false, nil),
	op("voice", "sendVoice", "Voice message", s("voice"), s("voice"), true, nil),
	op("paid-media", "sendPaidMedia", "Paid media collection", s("star_count", "media"), s("media"), false,
		Object{"star_count": 1, "media": []any{Object{"type": "photo", "media": sampleMedia()}}}),
	op("media-group", "sendMediaGroup", "Media album", s("media"), s("media"), true,
		Object{"media": []any{
			Object{"type": "photo", "media": sampleMedia()},
			Object{"type": "photo", "media": sampleMedia()},
		}}),
	op("location", "sendLocation", "Geographic location", s("latitude", "longitude"), nil, true,
		Object{"latitude": -6.2, "longitude": 106.8}),
	op("venue", "sendVenue", "Venue", s("latitude", "longitude", "title", "address"), nil, true,
		Object{"latitude": -6.2, "longitude": 106.8, "title": "Office", "address": "Jakarta"}),
	op("contact", "sendContact", "Contact", s("phone_number", "first_name"), nil, true,
		Object{"phone_number": "+621234567", "first_name": "Telesend"}),
	op("poll", "sendPoll", "Poll", s("question", "options"), nil, true,
		Object{"question": "Ready?", "options": []any{Object{"text": "Yes"}}}),
	op("checklist", "sendChecklist", "Business checklist", s("business_connection_id", "checklist"), nil, true,
		Object{"business_connection_id": "connection", "checklist": Object{
			"title": "Tasks", "tasks": []any{Object{"id": 1, "text": "Ship"}}}}),
	op("dice", "sendDice", "Animated dice", nil, nil, true, Object{}),
	op("invoice", "sendInvoice", "Invoice", s("title", "description", "payload", "currency", "prices"), nil, true,
		Object{"title": "Order", "description": "Telesend order", "payload": "order-1", "currency": "XTR",
			"prices": []any{Object{"label": "Order", "amount": 1}}}),
	op("game", "sendGame", "HTML5 game", s("game_short_name"), nil, true, Object{"game_short_name": "game"}),
}

// Get returns the operation for a message type.
func Get(typ string) (Operation, bool) {
	for _, o := range Catalog {
		if o.Type == typ {
			return o, true
		}
	}
	return Operation{}, false
}
