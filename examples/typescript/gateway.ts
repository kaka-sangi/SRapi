import { client } from '../../packages/sdk/typescript/src/client.gen';
import {
  countAnthropicMessageTokens,
  countGeminiTokens,
  createChatCompletion,
  createMessage,
  createResponse,
  listAdminOpsRealtimeSlots,
  listGeminiModels,
  listModels,
  type AnthropicCountTokensRequest,
  type AnthropicMessagesRequest,
  type ChatCompletionRequest,
  type GeminiCountTokensRequest,
  type ResponsesRequest,
} from '../../packages/sdk/typescript/src';

declare const process: {
  env: Record<string, string | undefined>;
  exit(code?: number): never;
};

const baseUrl = process.env.SRAPI_BASE_URL ?? 'http://127.0.0.1:8080';
const apiKey = requiredEnv('SRAPI_API_KEY');
const model = process.env.SRAPI_MODEL ?? 'gpt-4o-mini';
const geminiModel = process.env.SRAPI_GEMINI_MODEL ?? model;
const adminSession = process.env.SRAPI_ADMIN_SESSION;

client.setConfig({
  baseUrl,
  auth: (auth) => {
    if (auth.in === 'cookie' && auth.name === 'srapi_session') {
      return adminSession;
    }
    if (auth.scheme === 'bearer') {
      return apiKey;
    }
    return undefined;
  },
});

async function main() {
  const models = await listModels();
  print('GET /v1/models', models.data);

  const chatBody: ChatCompletionRequest = {
    model,
    messages: [{ role: 'user', content: 'hello from TypeScript chat' }],
    stream: false,
  };
  const chat = await createChatCompletion({ body: chatBody });
  print('POST /v1/chat/completions', chat.data);

  const responsesBody: ResponsesRequest = {
    model,
    input: 'hello from TypeScript responses',
    stream: false,
  };
  const response = await createResponse({ body: responsesBody });
  print('POST /v1/responses', response.data);

  const messageBody: AnthropicMessagesRequest = {
    model,
    max_tokens: 128,
    messages: [{ role: 'user', content: 'hello from TypeScript messages' }],
    stream: false,
  };
  const message = await createMessage({ body: messageBody });
  print('POST /v1/messages', message.data);

  const geminiModels = await listGeminiModels();
  print('GET /v1beta/models', geminiModels.data);

  const geminiCountBody: GeminiCountTokensRequest = {
    contents: [{ role: 'user', parts: [{ text: 'count this Gemini-compatible request' }] }],
  };
  const geminiCount = await countGeminiTokens({
    path: { model: geminiModel },
    body: geminiCountBody,
  });
  print('POST /v1beta/models/{model}:countTokens', geminiCount.data);

  const anthropicCountBody: AnthropicCountTokensRequest = {
    model,
    messages: [{ role: 'user', content: 'count this Anthropic-compatible request' }],
  };
  const anthropicCount = await countAnthropicMessageTokens({ body: anthropicCountBody });
  print('POST /v1/messages/count_tokens', anthropicCount.data);

  if (adminSession) {
    const slots = await listAdminOpsRealtimeSlots({ query: { page: 1, page_size: 20 } });
    print('GET /api/v1/admin/ops/realtime/slots', slots.data);
  }
}

function requiredEnv(name: string): string {
  const value = process.env[name];
  if (!value) {
    throw new Error(`${name} is required`);
  }
  return value;
}

function print(label: string, value: unknown) {
  console.log(label);
  console.log(JSON.stringify(value, null, 2));
}

main().catch((error) => {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
});
