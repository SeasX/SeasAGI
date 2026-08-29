import { useState } from 'react';

interface TranslatorPageProps {
  // Optional callbacks
}

export function TranslatorPage({}: TranslatorPageProps) {
  const [inputFormat, setInputFormat] = useState('chat_completions');
  const [outputFormat, setOutputFormat] = useState('responses');
  const [inputText, setInputText] = useState('');
  const [outputText, setOutputText] = useState('');
  const [error, setError] = useState('');

  const formatOptions = [
    { value: 'chat_completions', label: 'Chat Completions' },
    { value: 'responses', label: 'Responses API' },
    { value: 'gemini', label: 'Gemini' },
    { value: 'anthropic', label: 'Anthropic Messages' },
  ];

  const handleTranslate = () => {
    setError('');
    try {
      const parsed = JSON.parse(inputText);
      let result: unknown;

      if (inputFormat === 'chat_completions' && outputFormat === 'responses') {
        // Chat Completions → Responses API
        result = translateChatToResponses(parsed);
      } else if (inputFormat === 'responses' && outputFormat === 'chat_completions') {
        // Responses → Chat Completions
        result = translateResponsesToChat(parsed);
      } else if (inputFormat === outputFormat) {
        result = parsed;
      } else {
        // Generic passthrough for other combinations
        result = { ...parsed, _translated_to: outputFormat };
      }

      setOutputText(JSON.stringify(result, null, 2));
    } catch (e) {
      setError(`Invalid JSON: ${(e as Error).message}`);
      setOutputText('');
    }
  };

  const handleSwap = () => {
    const tmp = inputFormat;
    setInputFormat(outputFormat);
    setOutputFormat(tmp);
  };

  const loadExample = () => {
    if (inputFormat === 'chat_completions') {
      setInputText(JSON.stringify({
        model: 'gpt-4',
        messages: [
          { role: 'system', content: 'You are a helpful assistant.' },
          { role: 'user', content: 'Hello!' }
        ],
        stream: false
      }, null, 2));
    } else if (inputFormat === 'responses') {
      setInputText(JSON.stringify({
        model: 'gpt-4',
        input: 'Hello!',
        instructions: 'You are a helpful assistant.',
        stream: false
      }, null, 2));
    }
  };

  return (
    <div className="p-6 max-w-6xl mx-auto">
      <h1 className="text-2xl font-bold mb-2 text-zinc-900 dark:text-white">
        Translator Debug Panel
      </h1>
      <p className="text-zinc-500 dark:text-zinc-400 mb-6">
        Visualize and test API format translations between different LLM API formats.
      </p>

      {/* Format selectors */}
      <div className="flex items-center gap-3 mb-4">
        <select
          value={inputFormat}
          onChange={(e) => setInputFormat(e.target.value)}
          className="px-3 py-2 rounded-lg border border-zinc-300 dark:border-zinc-600 bg-white dark:bg-zinc-800 text-zinc-900 dark:text-white"
        >
          {formatOptions.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>

        <button
          onClick={handleSwap}
          className="p-2 rounded-lg bg-zinc-100 dark:bg-zinc-700 hover:bg-zinc-200 dark:hover:bg-zinc-600"
          title="Swap formats"
        >
          ⇄
        </button>

        <select
          value={outputFormat}
          onChange={(e) => setOutputFormat(e.target.value)}
          className="px-3 py-2 rounded-lg border border-zinc-300 dark:border-zinc-600 bg-white dark:bg-zinc-800 text-zinc-900 dark:text-white"
        >
          {formatOptions.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>
      </div>

      {/* Input/Output panels */}
      <div className="grid grid-cols-2 gap-4">
        <div>
          <div className="flex items-center justify-between mb-2">
            <label className="text-sm font-medium text-zinc-700 dark:text-zinc-300">
              Input ({inputFormat})
            </label>
            <button
              onClick={loadExample}
              className="text-xs text-blue-500 hover:text-blue-600"
            >
              Load Example
            </button>
          </div>
          <textarea
            value={inputText}
            onChange={(e) => setInputText(e.target.value)}
            placeholder="Paste JSON here..."
            className="w-full h-96 px-3 py-2 font-mono text-sm rounded-lg border border-zinc-300 dark:border-zinc-600 bg-white dark:bg-zinc-800 text-zinc-900 dark:text-white resize-none"
          />
        </div>

        <div>
          <label className="block text-sm font-medium text-zinc-700 dark:text-zinc-300 mb-2">
            Output ({outputFormat})
          </label>
          <pre className="w-full h-96 px-3 py-2 font-mono text-sm rounded-lg border border-zinc-300 dark:border-zinc-600 bg-zinc-50 dark:bg-zinc-900 text-zinc-900 dark:text-white overflow-auto">
            {outputText || 'Output will appear here...'}
          </pre>
        </div>
      </div>

      {error && (
        <div className="mt-3 p-3 bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-400 rounded-lg text-sm">
          {error}
        </div>
      )}

      <div className="mt-4 flex gap-2">
        <button
          onClick={handleTranslate}
          disabled={!inputText}
          className="px-6 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600 disabled:opacity-50"
        >
          Translate
        </button>
        <button
          onClick={() => { setInputText(''); setOutputText(''); setError(''); }}
          className="px-4 py-2 text-zinc-500 hover:text-zinc-700 border border-zinc-300 dark:border-zinc-600 rounded-lg"
        >
          Clear
        </button>
      </div>
    </div>
  );
}

// Translation helpers (frontend-side preview)
function translateChatToResponses(input: Record<string, unknown>): Record<string, unknown> {
  const messages = (input.messages as Array<Record<string, string>>) || [];
  const systemMsgs = messages.filter(m => m.role === 'system');
  const userMsgs = messages.filter(m => m.role === 'user');

  return {
    model: input.model,
    input: userMsgs.map(m => m.content).join('\n') || '',
    instructions: systemMsgs.map(m => m.content).join('\n') || undefined,
    stream: input.stream || false,
  };
}

function translateResponsesToChat(input: Record<string, unknown>): Record<string, unknown> {
  const messages: Array<Record<string, string>> = [];
  if (input.instructions) {
    messages.push({ role: 'system', content: input.instructions as string });
  }
  if (input.input) {
    messages.push({ role: 'user', content: input.input as string });
  }

  return {
    model: input.model,
    messages,
    stream: input.stream || false,
  };
}
