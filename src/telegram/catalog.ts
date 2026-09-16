export type MediaSourceKind = "file_id" | "path" | "upload" | "url";

export interface MediaSource {
  source: MediaSourceKind;
  value?: string;
  file?: Blob;
  filename?: string;
}

export interface MessageOperation {
  type: string;
  telegramMethod: string;
  description: string;
  requiredFields: readonly string[];
  mediaFields: readonly string[];
  allowUrl: boolean;
  examplePayload: Record<string, unknown>;
}

export const TELEGRAM_BOT_API_VERSION = "10.3";

export const MESSAGE_CATALOG = [
  operation("text", "sendMessage", "Text message", ["text"], [], true, { text: "Hello" }),
  operation("message-draft", "sendMessageDraft", "Streaming text draft", ["draft_id"], [], true, {
    draft_id: 1,
    text: "Thinking...",
  }),
  operation(
    "rich-message",
    "sendRichMessage",
    "Rich structured message",
    ["rich_message"],
    [],
    true,
    {
      rich_message: { markdown: "**Hello**" },
    },
  ),
  operation(
    "rich-message-draft",
    "sendRichMessageDraft",
    "Streaming rich message draft",
    ["draft_id", "rich_message"],
    [],
    false,
    { draft_id: 1, rich_message: { markdown: "**Thinking**" } },
  ),
  operation("animation", "sendAnimation", "Animation", ["animation"], ["animation"], true),
  operation("audio", "sendAudio", "Audio track", ["audio"], ["audio"], true),
  operation("document", "sendDocument", "Document", ["document"], ["document"], true),
  operation(
    "live-photo",
    "sendLivePhoto",
    "Live photo",
    ["live_photo", "photo"],
    ["live_photo", "photo"],
    false,
  ),
  operation("photo", "sendPhoto", "Photo", ["photo"], ["photo"], true),
  operation("sticker", "sendSticker", "Sticker", ["sticker"], ["sticker"], true),
  operation("video", "sendVideo", "Video", ["video"], ["video"], true),
  operation("video-note", "sendVideoNote", "Video note", ["video_note"], ["video_note"], false),
  operation("voice", "sendVoice", "Voice message", ["voice"], ["voice"], true),
  operation(
    "paid-media",
    "sendPaidMedia",
    "Paid media collection",
    ["star_count", "media"],
    ["media"],
    false,
    { star_count: 1, media: [{ type: "photo", media: sampleMedia() }] },
  ),
  operation("media-group", "sendMediaGroup", "Media album", ["media"], ["media"], true, {
    media: [
      { type: "photo", media: sampleMedia() },
      { type: "photo", media: sampleMedia() },
    ],
  }),
  operation(
    "location",
    "sendLocation",
    "Geographic location",
    ["latitude", "longitude"],
    [],
    true,
    {
      latitude: -6.2,
      longitude: 106.8,
    },
  ),
  operation(
    "venue",
    "sendVenue",
    "Venue",
    ["latitude", "longitude", "title", "address"],
    [],
    true,
    { latitude: -6.2, longitude: 106.8, title: "Office", address: "Jakarta" },
  ),
  operation("contact", "sendContact", "Contact", ["phone_number", "first_name"], [], true, {
    phone_number: "+621234567",
    first_name: "Telesend",
  }),
  operation("poll", "sendPoll", "Poll", ["question", "options"], [], true, {
    question: "Ready?",
    options: [{ text: "Yes" }],
  }),
  operation(
    "checklist",
    "sendChecklist",
    "Business checklist",
    ["business_connection_id", "checklist"],
    [],
    true,
    {
      business_connection_id: "connection",
      checklist: { title: "Tasks", tasks: [{ id: 1, text: "Ship" }] },
    },
  ),
  operation("dice", "sendDice", "Animated dice", [], [], true, {}),
  operation(
    "invoice",
    "sendInvoice",
    "Invoice",
    ["title", "description", "payload", "currency", "prices"],
    [],
    true,
    {
      title: "Order",
      description: "Telesend order",
      payload: "order-1",
      currency: "XTR",
      prices: [{ label: "Order", amount: 1 }],
    },
  ),
  operation("game", "sendGame", "HTML5 game", ["game_short_name"], [], true, {
    game_short_name: "game",
  }),
] as const satisfies readonly MessageOperation[];

function sampleMedia(): MediaSource {
  return { source: "file_id", value: "telegram-file-id" };
}

function operation(
  type: string,
  telegramMethod: string,
  description: string,
  requiredFields: readonly string[],
  mediaFields: readonly string[],
  allowUrl: boolean,
  examplePayload?: Record<string, unknown>,
): MessageOperation {
  const defaultPayload = Object.fromEntries(mediaFields.map((field) => [field, sampleMedia()]));
  return {
    type,
    telegramMethod,
    description,
    requiredFields,
    mediaFields,
    allowUrl,
    examplePayload: examplePayload ?? defaultPayload,
  };
}

export type MessageType = (typeof MESSAGE_CATALOG)[number]["type"];

export function getOperation(type: string): MessageOperation {
  const operation = MESSAGE_CATALOG.find((entry) => entry.type === type);
  if (!operation) throw new Error(`Unsupported message type: ${type}`);
  return operation;
}
