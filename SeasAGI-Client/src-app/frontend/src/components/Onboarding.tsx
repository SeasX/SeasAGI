import { useState } from 'react';

type OnboardingStep = 'language' | 'provider' | 'combo' | 'test' | 'done';

interface OnboardingProps {
  onComplete: () => void;
  onCancel: () => void;
}

export function Onboarding({ onComplete, onCancel }: OnboardingProps) {
  const [step, setStep] = useState<OnboardingStep>('language');
  const [language, setLanguage] = useState('en');
  const [provider, setProvider] = useState('');
  const [apiKey, setApiKey] = useState('');
  const [comboName, setComboName] = useState('');
  const [testResult, setTestResult] = useState<'pending' | 'success' | 'fail'>('pending');

  const steps: OnboardingStep[] = ['language', 'provider', 'combo', 'test', 'done'];
  const currentIdx = steps.indexOf(step);

  const handleNext = () => {
    const nextIdx = currentIdx + 1;
    if (nextIdx < steps.length) {
      setStep(steps[nextIdx]);
    }
  };

  const handleBack = () => {
    const prevIdx = currentIdx - 1;
    if (prevIdx >= 0) {
      setStep(steps[prevIdx]);
    }
  };

  const handleTest = () => {
    if (apiKey && provider) {
      setTestResult('success');
    } else {
      setTestResult('fail');
    }
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white dark:bg-zinc-900 rounded-2xl p-8 max-w-lg w-full mx-4 shadow-2xl">
        <h2 className="text-2xl font-bold mb-2 text-zinc-900 dark:text-white">
          Welcome to SeasAGI
        </h2>
        <p className="text-zinc-500 dark:text-zinc-400 mb-6">
          Let's set up your AI gateway in a few steps.
        </p>

        {/* Progress bar */}
        <div className="flex gap-2 mb-6">
          {steps.slice(0, -1).map((s, i) => (
            <div
              key={s}
              className={`h-1.5 flex-1 rounded-full transition-colors ${
                i <= currentIdx ? 'bg-blue-500' : 'bg-zinc-200 dark:bg-zinc-700'
              }`}
            />
          ))}
        </div>

        {step === 'language' && (
          <div className="space-y-4">
            <label className="block text-sm font-medium text-zinc-700 dark:text-zinc-300">
              Select Language / 选择语言
            </label>
            <select
              value={language}
              onChange={(e) => setLanguage(e.target.value)}
              className="w-full px-3 py-2 rounded-lg border border-zinc-300 dark:border-zinc-600 bg-white dark:bg-zinc-800 text-zinc-900 dark:text-white"
            >
              <option value="en">English</option>
              <option value="zh-CN">简体中文</option>
              <option value="ja">日本語</option>
              <option value="ko">한국어</option>
            </select>
            <div className="flex justify-end gap-2 pt-4">
              <button onClick={onCancel} className="px-4 py-2 text-zinc-500 hover:text-zinc-700">
                Skip
              </button>
              <button
                onClick={handleNext}
                className="px-6 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600"
              >
                Next
              </button>
            </div>
          </div>
        )}

        {step === 'provider' && (
          <div className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-zinc-700 dark:text-zinc-300 mb-1">
                AI Provider
              </label>
              <select
                value={provider}
                onChange={(e) => setProvider(e.target.value)}
                className="w-full px-3 py-2 rounded-lg border border-zinc-300 dark:border-zinc-600 bg-white dark:bg-zinc-800 text-zinc-900 dark:text-white"
              >
                <option value="">Select a provider...</option>
                <option value="openai">OpenAI</option>
                <option value="anthropic">Anthropic</option>
                <option value="google">Google Gemini</option>
                <option value="deepseek">DeepSeek</option>
                <option value="ollama">Ollama (Local)</option>
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium text-zinc-700 dark:text-zinc-300 mb-1">
                API Key
              </label>
              <input
                type="password"
                value={apiKey}
                onChange={(e) => setApiKey(e.target.value)}
                placeholder="sk-..."
                className="w-full px-3 py-2 rounded-lg border border-zinc-300 dark:border-zinc-600 bg-white dark:bg-zinc-800 text-zinc-900 dark:text-white"
              />
            </div>
            <div className="flex justify-between gap-2 pt-4">
              <button onClick={handleBack} className="px-4 py-2 text-zinc-500 hover:text-zinc-700">
                Back
              </button>
              <button
                onClick={handleNext}
                disabled={!provider}
                className="px-6 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600 disabled:opacity-50"
              >
                Next
              </button>
            </div>
          </div>
        )}

        {step === 'combo' && (
          <div className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-zinc-700 dark:text-zinc-300 mb-1">
                Combo Name
              </label>
              <input
                type="text"
                value={comboName}
                onChange={(e) => setComboName(e.target.value)}
                placeholder="My Default Combo"
                className="w-full px-3 py-2 rounded-lg border border-zinc-300 dark:border-zinc-600 bg-white dark:bg-zinc-800 text-zinc-900 dark:text-white"
              />
              <p className="text-xs text-zinc-400 mt-1">
                A Combo groups multiple providers with routing rules.
              </p>
            </div>
            <div className="flex justify-between gap-2 pt-4">
              <button onClick={handleBack} className="px-4 py-2 text-zinc-500 hover:text-zinc-700">
                Back
              </button>
              <button
                onClick={handleNext}
                disabled={!comboName}
                className="px-6 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600 disabled:opacity-50"
              >
                Next
              </button>
            </div>
          </div>
        )}

        {step === 'test' && (
          <div className="space-y-4">
            <p className="text-sm text-zinc-600 dark:text-zinc-400">
              Let's verify your configuration works.
            </p>
            <button
              onClick={handleTest}
              className="w-full px-4 py-3 bg-blue-500 text-white rounded-lg hover:bg-blue-600"
            >
              Run Test Request
            </button>
            {testResult === 'success' && (
              <div className="p-3 bg-green-50 dark:bg-green-900/20 text-green-700 dark:text-green-400 rounded-lg text-sm">
                ✓ Test successful! Your gateway is ready.
              </div>
            )}
            {testResult === 'fail' && (
              <div className="p-3 bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-400 rounded-lg text-sm">
                ✗ Test failed. Please check your API key and provider settings.
              </div>
            )}
            <div className="flex justify-between gap-2 pt-4">
              <button onClick={handleBack} className="px-4 py-2 text-zinc-500 hover:text-zinc-700">
                Back
              </button>
              <button
                onClick={handleNext}
                disabled={testResult !== 'success'}
                className="px-6 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600 disabled:opacity-50"
              >
                Finish
              </button>
            </div>
          </div>
        )}

        {step === 'done' && (
          <div className="space-y-4 text-center">
            <div className="text-4xl">✓</div>
            <h3 className="text-xl font-bold text-zinc-900 dark:text-white">
              Setup Complete!
            </h3>
            <p className="text-sm text-zinc-500 dark:text-zinc-400">
              Your SeasAGI gateway is configured and ready to use.
            </p>
            <button
              onClick={onComplete}
              className="px-8 py-3 bg-blue-500 text-white rounded-lg hover:bg-blue-600"
            >
              Start Using SeasAGI
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
