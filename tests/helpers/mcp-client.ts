import {
  InMemoryTransport,
  type JSONRPCMessage,
  type JSONRPCResponse,
  LATEST_PROTOCOL_VERSION,
  type McpServer,
} from "@modelcontextprotocol/server";

export class TestMcpClient {
  private id = 0;
  private readonly pending = new Map<
    number,
    { resolve: (value: unknown) => void; reject: (error: Error) => void }
  >();

  private constructor(private readonly transport: InMemoryTransport) {
    transport.onmessage = (message) => this.receive(message);
  }

  static async connect(server: McpServer): Promise<TestMcpClient> {
    const [clientTransport, serverTransport] = InMemoryTransport.createLinkedPair();
    const client = new TestMcpClient(clientTransport);
    await clientTransport.start();
    await server.connect(serverTransport);
    await client.request("initialize", {
      protocolVersion: LATEST_PROTOCOL_VERSION,
      capabilities: {},
      clientInfo: { name: "telesend-test", version: "1" },
    });
    await clientTransport.send({ jsonrpc: "2.0", method: "notifications/initialized" });
    return client;
  }

  async request(method: string, params: Record<string, unknown>): Promise<unknown> {
    const id = ++this.id;
    const response = new Promise<unknown>((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
    });
    await this.transport.send({ jsonrpc: "2.0", id, method, params } as JSONRPCMessage);
    return response;
  }

  async call(
    name: string,
    args: Record<string, unknown>,
  ): Promise<{ isError?: boolean; structuredContent?: Record<string, unknown> }> {
    return (await this.request("tools/call", { name, arguments: args })) as {
      isError?: boolean;
      structuredContent?: Record<string, unknown>;
    };
  }

  private receive(message: JSONRPCMessage): void {
    if (!("id" in message) || !("result" in message || "error" in message)) return;
    const response = message as JSONRPCResponse;
    const pending = this.pending.get(Number(response.id));
    if (!pending) return;
    this.pending.delete(Number(response.id));
    if ("error" in response) pending.reject(new Error(response.error.message));
    else pending.resolve(response.result);
  }
}
