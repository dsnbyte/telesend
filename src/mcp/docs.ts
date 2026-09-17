/**
 * Static documentation for Telegram Bot API advanced parameters.
 *
 * Common parameters (disable_notification, parse_mode, caption) are documented
 * inline in each MCP tool schema. This module covers advanced and rarely-used
 * parameters so agents can self-serve without opening the Telegram Bot API website.
 *
 * Updated for Bot API v10.3.
 */

export interface ParamDoc {
  name: string;
  type: string;
  required: boolean;
  description: string;
}

export interface MethodDoc {
  /** Telegram Bot API method name */
  method: string;
  /** MCP tool that exposes this method */
  mcpTool: string;
  /** Brief description of the method */
  description: string;
  /** Advanced parameters not shown in the MCP tool schema */
  advancedParams: ParamDoc[];
}

// ─── Shared param snippets ────────────────────────────────────────────────────

const REPLY_PARAMETERS: ParamDoc = {
  name: "reply_parameters",
  type: "object",
  required: false,
  description:
    "Reply to another message. Shape: { message_id: integer, chat_id?: string|integer, allow_sending_without_reply?: boolean, quote?: string, quote_parse_mode?: string, quote_entities?: MessageEntity[], quote_position?: integer }",
};

const REPLY_MARKUP: ParamDoc = {
  name: "reply_markup",
  type: "InlineKeyboardMarkup | ReplyKeyboardMarkup | ReplyKeyboardRemove | ForceReply",
  required: false,
  description:
    "Attach a keyboard or force reply. InlineKeyboardMarkup: { inline_keyboard: [[{ text, callback_data|url|... }]] }. ReplyKeyboardMarkup: { keyboard: [[{ text }]], resize_keyboard?, one_time_keyboard? }. ReplyKeyboardRemove: { remove_keyboard: true }. ForceReply: { force_reply: true }.",
};

const PROTECT_CONTENT: ParamDoc = {
  name: "protect_content",
  type: "boolean",
  required: false,
  description: "Protects the message from being forwarded or saved by recipients.",
};

const MESSAGE_EFFECT_ID: ParamDoc = {
  name: "message_effect_id",
  type: "string",
  required: false,
  description:
    "Unique identifier of the message effect (animated emoji overlay). Available effects: 🔥 5104841245755899700, 👍 5107584321108051014, 🎉 5046509860389126442, ❤️ 5044134455711629726, 💊 5109179936568840721, 💩 5046589136895476101.",
};

const BUSINESS_CONNECTION_ID: ParamDoc = {
  name: "business_connection_id",
  type: "string",
  required: false,
  description:
    "Unique identifier of the business connection on behalf of which the message will be sent. Required for business account bots.",
};

const MESSAGE_THREAD_ID_DOC: ParamDoc = {
  name: "message_thread_id",
  type: "integer",
  required: false,
  description:
    "Unique identifier for the target message thread (topic) in a forum supergroup. Passed via the top-level messageThreadId MCP parameter.",
};

const CAPTION_PARAMS: ParamDoc[] = [
  {
    name: "caption_entities",
    type: "MessageEntity[]",
    required: false,
    description:
      "Special entities in the caption (bold, italic, links…). Alternative to caption + parse_mode.",
  },
  {
    name: "show_caption_above_media",
    type: "boolean",
    required: false,
    description: "Pass true to show the caption above the media (photo/video only).",
  },
];

const HAS_SPOILER: ParamDoc = {
  name: "has_spoiler",
  type: "boolean",
  required: false,
  description: "Pass true to cover the media with a spoiler animation.",
};

const THUMBNAIL: ParamDoc = {
  name: "thumbnail",
  type: "MediaSource",
  required: false,
  description:
    "Thumbnail for video, audio, animation, or document. Must be a local path (source: path). Shape: { source: 'path', value: '/path/to/thumb.jpg' }. JPEG only, max 200 kB, 320×320 px.",
};

// ─── Method docs ─────────────────────────────────────────────────────────────

export const TELEGRAM_DOCS: Record<string, MethodDoc> = {
  sendMessage: {
    method: "sendMessage",
    mcpTool: "send_text",
    description: "Send a plain text message to a chat.",
    advancedParams: [
      {
        name: "entities",
        type: "MessageEntity[]",
        required: false,
        description:
          "Pre-parsed text entities (bold, italic, code, mention, url…). Use instead of parse_mode when you control entity offsets precisely. Each entity: { type, offset, length, url?, user?, language?, custom_emoji_id? }.",
      },
      {
        name: "link_preview_options",
        type: "object",
        required: false,
        description:
          "Control link preview. Shape: { is_disabled?: boolean, url?: string, prefer_small_media?: boolean, prefer_large_media?: boolean, show_above_text?: boolean }.",
      },
      REPLY_PARAMETERS,
      REPLY_MARKUP,
      PROTECT_CONTENT,
      MESSAGE_EFFECT_ID,
      BUSINESS_CONNECTION_ID,
      MESSAGE_THREAD_ID_DOC,
    ],
  },

  sendMessageDraft: {
    method: "sendMessageDraft",
    mcpTool: "send_text",
    description:
      "Send or update a streaming text draft (thinking/processing indicator). Use draft_id in send_text payload to activate.",
    advancedParams: [
      {
        name: "text",
        type: "string",
        required: false,
        description:
          "Current text content of the draft. Can be updated by resending with the same draft_id.",
      },
      {
        name: "entities",
        type: "MessageEntity[]",
        required: false,
        description: "Text entities for the draft content.",
      },
      REPLY_PARAMETERS,
      PROTECT_CONTENT,
      BUSINESS_CONNECTION_ID,
      MESSAGE_THREAD_ID_DOC,
    ],
  },

  sendRichMessage: {
    method: "sendRichMessage",
    mcpTool: "send_rich_message",
    description:
      "Send a rich/structured message. The rich_message object supports markdown, blocks, and structured layouts.",
    advancedParams: [
      {
        name: "rich_message",
        type: "object",
        required: true,
        description:
          "Rich message payload. Supported shapes: { markdown: string } for Telegram-flavored markdown; { text: string, parse_mode: string } for HTML/MarkdownV2; structured block objects per Bot API spec.",
      },
      REPLY_PARAMETERS,
      REPLY_MARKUP,
      PROTECT_CONTENT,
      MESSAGE_EFFECT_ID,
      BUSINESS_CONNECTION_ID,
      MESSAGE_THREAD_ID_DOC,
    ],
  },

  sendRichMessageDraft: {
    method: "sendRichMessageDraft",
    mcpTool: "send_rich_message",
    description:
      "Send or update a streaming rich-message draft. Use draft_id in send_rich_message payload to activate.",
    advancedParams: [
      {
        name: "rich_message",
        type: "object",
        required: true,
        description: "Current rich message content of the draft.",
      },
      REPLY_PARAMETERS,
      PROTECT_CONTENT,
      BUSINESS_CONNECTION_ID,
      MESSAGE_THREAD_ID_DOC,
    ],
  },

  sendPhoto: {
    method: "sendPhoto",
    mcpTool: "send_media",
    description: "Send a photo. Use type: 'photo' in send_media.",
    advancedParams: [
      ...CAPTION_PARAMS,
      HAS_SPOILER,
      {
        name: "show_caption_above_media",
        type: "boolean",
        required: false,
        description: "Show caption above the photo instead of below.",
      },
      REPLY_PARAMETERS,
      REPLY_MARKUP,
      PROTECT_CONTENT,
      MESSAGE_EFFECT_ID,
      BUSINESS_CONNECTION_ID,
    ],
  },

  sendVideo: {
    method: "sendVideo",
    mcpTool: "send_media",
    description: "Send a video file. Use type: 'video' in send_media.",
    advancedParams: [
      {
        name: "duration",
        type: "integer",
        required: false,
        description: "Video duration in seconds.",
      },
      {
        name: "width",
        type: "integer",
        required: false,
        description: "Video width in pixels.",
      },
      {
        name: "height",
        type: "integer",
        required: false,
        description: "Video height in pixels.",
      },
      THUMBNAIL,
      ...CAPTION_PARAMS,
      HAS_SPOILER,
      {
        name: "supports_streaming",
        type: "boolean",
        required: false,
        description: "Pass true if the uploaded video supports streaming.",
      },
      REPLY_PARAMETERS,
      REPLY_MARKUP,
      PROTECT_CONTENT,
      MESSAGE_EFFECT_ID,
      BUSINESS_CONNECTION_ID,
    ],
  },

  sendAnimation: {
    method: "sendAnimation",
    mcpTool: "send_media",
    description:
      "Send a GIF or H.264/MPEG-4 AVC animation without sound. Use type: 'animation' in send_media.",
    advancedParams: [
      {
        name: "duration",
        type: "integer",
        required: false,
        description: "Animation duration in seconds.",
      },
      {
        name: "width",
        type: "integer",
        required: false,
        description: "Animation width.",
      },
      {
        name: "height",
        type: "integer",
        required: false,
        description: "Animation height.",
      },
      THUMBNAIL,
      ...CAPTION_PARAMS,
      HAS_SPOILER,
      REPLY_PARAMETERS,
      REPLY_MARKUP,
      PROTECT_CONTENT,
      MESSAGE_EFFECT_ID,
      BUSINESS_CONNECTION_ID,
    ],
  },

  sendAudio: {
    method: "sendAudio",
    mcpTool: "send_media",
    description: "Send an audio track shown in the music player. Use type: 'audio' in send_media.",
    advancedParams: [
      {
        name: "duration",
        type: "integer",
        required: false,
        description: "Duration of the audio in seconds.",
      },
      {
        name: "performer",
        type: "string",
        required: false,
        description: "Performer name shown in the music player.",
      },
      {
        name: "title",
        type: "string",
        required: false,
        description: "Track title shown in the music player.",
      },
      THUMBNAIL,
      ...CAPTION_PARAMS,
      REPLY_PARAMETERS,
      REPLY_MARKUP,
      PROTECT_CONTENT,
      MESSAGE_EFFECT_ID,
      BUSINESS_CONNECTION_ID,
    ],
  },

  sendDocument: {
    method: "sendDocument",
    mcpTool: "send_media",
    description: "Send a general file. Use type: 'document' in send_media.",
    advancedParams: [
      THUMBNAIL,
      ...CAPTION_PARAMS,
      {
        name: "disable_content_type_detection",
        type: "boolean",
        required: false,
        description:
          "Disables automatic server-side content type detection for files uploaded as document. Useful when file extension is wrong.",
      },
      REPLY_PARAMETERS,
      REPLY_MARKUP,
      PROTECT_CONTENT,
      MESSAGE_EFFECT_ID,
      BUSINESS_CONNECTION_ID,
    ],
  },

  sendSticker: {
    method: "sendSticker",
    mcpTool: "send_media",
    description: "Send a .WEBP, .TGS, or .WEBM sticker. Use type: 'sticker' in send_media.",
    advancedParams: [
      {
        name: "emoji",
        type: "string",
        required: false,
        description: "Emoji associated with the sticker. Only for uploaded .WEBP stickers.",
      },
      REPLY_PARAMETERS,
      REPLY_MARKUP,
      PROTECT_CONTENT,
      MESSAGE_EFFECT_ID,
      BUSINESS_CONNECTION_ID,
    ],
  },

  sendVoice: {
    method: "sendVoice",
    mcpTool: "send_media",
    description:
      "Send an audio file as a voice message (OGG/OPUS). Use type: 'voice' in send_media.",
    advancedParams: [
      {
        name: "duration",
        type: "integer",
        required: false,
        description: "Duration of the voice message in seconds.",
      },
      ...CAPTION_PARAMS,
      REPLY_PARAMETERS,
      REPLY_MARKUP,
      PROTECT_CONTENT,
      MESSAGE_EFFECT_ID,
      BUSINESS_CONNECTION_ID,
    ],
  },

  sendVideoNote: {
    method: "sendVideoNote",
    mcpTool: "send_media",
    description:
      "Send a round video note (max 1 min). Use type: 'video_note' in send_media. URL not supported.",
    advancedParams: [
      {
        name: "duration",
        type: "integer",
        required: false,
        description: "Duration of the video note in seconds.",
      },
      {
        name: "length",
        type: "integer",
        required: false,
        description: "Video width and height (must be equal since video notes are round).",
      },
      THUMBNAIL,
      REPLY_PARAMETERS,
      REPLY_MARKUP,
      PROTECT_CONTENT,
      MESSAGE_EFFECT_ID,
      BUSINESS_CONNECTION_ID,
    ],
  },

  sendLivePhoto: {
    method: "sendLivePhoto",
    mcpTool: "send_media",
    description:
      "Send a live photo (video + still frame). Use type: 'live_photo' in send_media. Requires both file (video) and photo (still). URL not supported.",
    advancedParams: [
      ...CAPTION_PARAMS,
      REPLY_PARAMETERS,
      REPLY_MARKUP,
      PROTECT_CONTENT,
      MESSAGE_EFFECT_ID,
      BUSINESS_CONNECTION_ID,
    ],
  },

  sendMediaGroup: {
    method: "sendMediaGroup",
    mcpTool: "send_media_group",
    description: "Send 2–10 media files as an album. Omit star_count in send_media_group.",
    advancedParams: [
      {
        name: "media[].caption",
        type: "string",
        required: false,
        description:
          "Caption for an individual album item. Only the first item's caption is shown.",
      },
      {
        name: "media[].parse_mode",
        type: "string",
        required: false,
        description: "Parse mode for the individual item caption.",
      },
      {
        name: "media[].has_spoiler",
        type: "boolean",
        required: false,
        description: "Spoiler animation on an individual photo or video in the album.",
      },
      {
        name: "media[].width",
        type: "integer",
        required: false,
        description: "Width of the video item.",
      },
      {
        name: "media[].height",
        type: "integer",
        required: false,
        description: "Height of the video item.",
      },
      {
        name: "media[].duration",
        type: "integer",
        required: false,
        description: "Duration of the video or audio item in seconds.",
      },
      {
        name: "media[].thumbnail",
        type: "MediaSource",
        required: false,
        description: "Thumbnail for a video item. Shape: { source: 'path', value: '...' }.",
      },
      REPLY_PARAMETERS,
      PROTECT_CONTENT,
      MESSAGE_EFFECT_ID,
      BUSINESS_CONNECTION_ID,
    ],
  },

  sendPaidMedia: {
    method: "sendPaidMedia",
    mcpTool: "send_media_group",
    description:
      "Send media locked behind Telegram Stars. Set star_count in send_media_group to activate.",
    advancedParams: [
      {
        name: "caption",
        type: "string",
        required: false,
        description: "Caption displayed below the paid media preview.",
      },
      ...CAPTION_PARAMS,
      {
        name: "show_caption_above_media",
        type: "boolean",
        required: false,
        description: "Show caption above the paid media preview.",
      },
      REPLY_PARAMETERS,
      PROTECT_CONTENT,
      BUSINESS_CONNECTION_ID,
    ],
  },

  sendLocation: {
    method: "sendLocation",
    mcpTool: "send_location",
    description: "Send a GPS location point. Use send_location without title/address.",
    advancedParams: [
      {
        name: "horizontal_accuracy",
        type: "number",
        required: false,
        description: "Radius of uncertainty for the location in meters (0–1500).",
      },
      {
        name: "live_period",
        type: "integer",
        required: false,
        description:
          "Period in seconds during which the location will be updated (60–86400). Creates a live location.",
      },
      {
        name: "heading",
        type: "integer",
        required: false,
        description: "Direction of movement in degrees (1–360). For live locations.",
      },
      {
        name: "proximity_alert_radius",
        type: "integer",
        required: false,
        description: "Maximum proximity alert distance in meters for live locations (1–100000).",
      },
      REPLY_PARAMETERS,
      REPLY_MARKUP,
      PROTECT_CONTENT,
      MESSAGE_EFFECT_ID,
      BUSINESS_CONNECTION_ID,
    ],
  },

  sendVenue: {
    method: "sendVenue",
    mcpTool: "send_location",
    description:
      "Send a venue with map pin. Use send_location with title and address to activate venue mode.",
    advancedParams: [
      {
        name: "foursquare_id",
        type: "string",
        required: false,
        description: "Foursquare identifier of the venue.",
      },
      {
        name: "foursquare_type",
        type: "string",
        required: false,
        description: "Foursquare type of the venue (e.g. arts_entertainment/default).",
      },
      {
        name: "google_place_id",
        type: "string",
        required: false,
        description: "Google Places identifier of the venue.",
      },
      {
        name: "google_place_type",
        type: "string",
        required: false,
        description:
          "Google Places type of the venue (see https://developers.google.com/maps/documentation/places/web-service/supported_types).",
      },
      REPLY_PARAMETERS,
      REPLY_MARKUP,
      PROTECT_CONTENT,
      MESSAGE_EFFECT_ID,
      BUSINESS_CONNECTION_ID,
    ],
  },

  sendContact: {
    method: "sendContact",
    mcpTool: "send_contact",
    description: "Send a phone contact card.",
    advancedParams: [
      {
        name: "last_name",
        type: "string",
        required: false,
        description: "Contact's last name.",
      },
      {
        name: "vcard",
        type: "string",
        required: false,
        description: "Additional contact info in vCard format (0–2048 bytes).",
      },
      REPLY_PARAMETERS,
      REPLY_MARKUP,
      PROTECT_CONTENT,
      MESSAGE_EFFECT_ID,
      BUSINESS_CONNECTION_ID,
    ],
  },

  sendPoll: {
    method: "sendPoll",
    mcpTool: "send_interactive",
    description: "Send a native poll. Use type: 'poll' in send_interactive.",
    advancedParams: [
      {
        name: "is_anonymous",
        type: "boolean",
        required: false,
        description: "Pass false to make the poll non-anonymous. Default true.",
      },
      {
        name: "type",
        type: "string",
        required: false,
        description: "Poll type: 'regular' (default) or 'quiz'.",
      },
      {
        name: "allows_multiple_answers",
        type: "boolean",
        required: false,
        description: "Pass true to allow multiple answers (regular polls only).",
      },
      {
        name: "correct_option_id",
        type: "integer",
        required: false,
        description: "0-based index of the correct answer (quiz polls only).",
      },
      {
        name: "explanation",
        type: "string",
        required: false,
        description: "Explanation shown after a wrong quiz answer (1–200 chars).",
      },
      {
        name: "explanation_parse_mode",
        type: "string",
        required: false,
        description: "Parse mode for the explanation: HTML or MarkdownV2.",
      },
      {
        name: "open_period",
        type: "integer",
        required: false,
        description:
          "Seconds the poll will be active after creation (5–600). Mutually exclusive with close_date.",
      },
      {
        name: "close_date",
        type: "integer",
        required: false,
        description:
          "Unix timestamp when the poll closes (now + 5s to now + 600s). Mutually exclusive with open_period.",
      },
      {
        name: "is_closed",
        type: "boolean",
        required: false,
        description: "Pass true to immediately close the poll.",
      },
      {
        name: "question_entities",
        type: "MessageEntity[]",
        required: false,
        description: "Special entities in the poll question. Alternative to question + parse_mode.",
      },
      {
        name: "question_parse_mode",
        type: "string",
        required: false,
        description: "Parse mode for the question text.",
      },
      REPLY_PARAMETERS,
      REPLY_MARKUP,
      PROTECT_CONTENT,
      MESSAGE_EFFECT_ID,
      BUSINESS_CONNECTION_ID,
    ],
  },

  sendChecklist: {
    method: "sendChecklist",
    mcpTool: "send_interactive",
    description:
      "Send a business checklist. Use type: 'checklist' in send_interactive. Requires a business connection.",
    advancedParams: [
      {
        name: "checklist",
        type: "object",
        required: true,
        description:
          "Checklist object. Shape: { title: string, tasks: [{ id: integer, text: string }], others_can_add_tasks?: boolean, others_can_mark_tasks_as_done?: boolean }.",
      },
      REPLY_PARAMETERS,
      BUSINESS_CONNECTION_ID,
    ],
  },

  sendDice: {
    method: "sendDice",
    mcpTool: "send_interactive",
    description: "Send an animated dice emoji. Use type: 'dice' in send_interactive.",
    advancedParams: [
      {
        name: "emoji",
        type: "string",
        required: false,
        description:
          "Emoji to use for the dice animation. Options: 🎲 (1–6, default), 🎯 (1–6), 🏀 (1–5), ⚽ (1–5), 🎳 (1–6), 🎰 (1–64 slot machine).",
      },
      REPLY_PARAMETERS,
      REPLY_MARKUP,
      PROTECT_CONTENT,
      MESSAGE_EFFECT_ID,
      BUSINESS_CONNECTION_ID,
    ],
  },

  sendInvoice: {
    method: "sendInvoice",
    mcpTool: "send_invoice",
    description: "Send a payment invoice.",
    advancedParams: [
      {
        name: "provider_token",
        type: "string",
        required: false,
        description:
          "Payment provider token from @BotFather. Omit for Telegram Stars payments (currency: XTR).",
      },
      {
        name: "max_tip_amount",
        type: "integer",
        required: false,
        description: "Maximum accepted tip amount in smallest currency units.",
      },
      {
        name: "suggested_tip_amounts",
        type: "integer[]",
        required: false,
        description: "Array of up to 4 suggested tip amounts.",
      },
      {
        name: "provider_data",
        type: "string",
        required: false,
        description: "JSON-serialized data for the payment provider (e.g. Stripe-specific params).",
      },
      {
        name: "photo_url",
        type: "string",
        required: false,
        description: "URL of the product photo for the invoice.",
      },
      {
        name: "photo_size",
        type: "integer",
        required: false,
        description: "Photo size in bytes.",
      },
      {
        name: "photo_width",
        type: "integer",
        required: false,
        description: "Photo width.",
      },
      {
        name: "photo_height",
        type: "integer",
        required: false,
        description: "Photo height.",
      },
      {
        name: "need_name",
        type: "boolean",
        required: false,
        description: "Require customer's full name.",
      },
      {
        name: "need_phone_number",
        type: "boolean",
        required: false,
        description: "Require customer's phone number.",
      },
      {
        name: "need_email",
        type: "boolean",
        required: false,
        description: "Require customer's email address.",
      },
      {
        name: "need_shipping_address",
        type: "boolean",
        required: false,
        description: "Require customer's shipping address.",
      },
      {
        name: "send_phone_number_to_provider",
        type: "boolean",
        required: false,
        description: "Pass customer phone number to the provider.",
      },
      {
        name: "send_email_to_provider",
        type: "boolean",
        required: false,
        description: "Pass customer email to the provider.",
      },
      {
        name: "is_flexible",
        type: "boolean",
        required: false,
        description: "Pass true if the final price depends on the shipping method.",
      },
      REPLY_PARAMETERS,
      REPLY_MARKUP,
      PROTECT_CONTENT,
      MESSAGE_EFFECT_ID,
      MESSAGE_THREAD_ID_DOC,
    ],
  },

  sendGame: {
    method: "sendGame",
    mcpTool: "send_interactive",
    description: "Send an HTML5 game. Use type: 'game' in send_interactive.",
    advancedParams: [
      REPLY_PARAMETERS,
      REPLY_MARKUP,
      PROTECT_CONTENT,
      MESSAGE_EFFECT_ID,
      BUSINESS_CONNECTION_ID,
    ],
  },
};

// ─── Lookup helpers ───────────────────────────────────────────────────────────

/** Maps Telesend message type names to Telegram method names */
const TYPE_TO_METHOD: Record<string, string> = {
  text: "sendMessage",
  "message-draft": "sendMessageDraft",
  "rich-message": "sendRichMessage",
  "rich-message-draft": "sendRichMessageDraft",
  photo: "sendPhoto",
  video: "sendVideo",
  animation: "sendAnimation",
  audio: "sendAudio",
  document: "sendDocument",
  sticker: "sendSticker",
  voice: "sendVoice",
  "video-note": "sendVideoNote",
  video_note: "sendVideoNote",
  "live-photo": "sendLivePhoto",
  live_photo: "sendLivePhoto",
  "media-group": "sendMediaGroup",
  media_group: "sendMediaGroup",
  "paid-media": "sendPaidMedia",
  paid_media: "sendPaidMedia",
  location: "sendLocation",
  venue: "sendVenue",
  contact: "sendContact",
  poll: "sendPoll",
  checklist: "sendChecklist",
  dice: "sendDice",
  invoice: "sendInvoice",
  game: "sendGame",
};

/**
 * Look up documentation by Telegram method name or Telesend message type.
 * Examples: getMethodDoc("sendMessage"), getMethodDoc("text"), getMethodDoc("photo")
 */
export function getMethodDoc(query: string): MethodDoc | undefined {
  // Direct Telegram method lookup (e.g. "sendMessage")
  if (query in TELEGRAM_DOCS) return TELEGRAM_DOCS[query];
  // Telesend type lookup (e.g. "text", "video_note")
  const method = TYPE_TO_METHOD[query];
  if (method) return TELEGRAM_DOCS[method];
  return undefined;
}

/** All valid query strings (for error messages / hints) */
export const KNOWN_QUERIES = [...Object.keys(TELEGRAM_DOCS), ...Object.keys(TYPE_TO_METHOD)];
