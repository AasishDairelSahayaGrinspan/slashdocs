import { useState, useRef, useCallback } from "react";
import { Send, Trash2, Bot } from "lucide-react";
import { Button } from "@/components/ui/button";
import { agentChat, type AgentMessage } from "@/api";
import { useDoc } from "./doc-context";

type Message = AgentMessage & { id: string };

let msgCounter = 0;
function nextId() {
  return String(++msgCounter);
}

export function AgentPanel({ mobile }: { mobile?: boolean }) {
  const { docId, version } = useDoc();
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState("");
  const [streaming, setStreaming] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const bottomRef = useRef<HTMLDivElement>(null);

  const scrollBottom = useCallback(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, []);

  const send = useCallback(
    async (text: string) => {
      if (!text.trim() || streaming) return;
      setError(null);

      const userMsg: Message = { id: nextId(), role: "user", content: text };
      const assistantId = nextId();

      setMessages((prev) => [
        ...prev,
        userMsg,
        { id: assistantId, role: "assistant", content: "" },
      ]);
      setInput("");
      setStreaming(true);
      scrollBottom();

      const history: AgentMessage[] = [
        ...messages.map(({ role, content }) => ({ role, content })),
        { role: "user", content: text },
      ];

      try {
        for await (const chunk of agentChat(docId, history, version)) {
          setMessages((prev) =>
            prev.map((m) =>
              m.id === assistantId ? { ...m, content: m.content + chunk } : m,
            ),
          );
          scrollBottom();
        }
      } catch (e) {
        const msg = e instanceof Error ? e.message : "Something went wrong";
        setError(msg);
        setMessages((prev) =>
          prev.filter((m) => m.id !== assistantId || m.content !== ""),
        );
      } finally {
        setStreaming(false);
      }
    },
    [docId, version, messages, streaming, scrollBottom],
  );

  const clear = useCallback(() => {
    setMessages([]);
    setError(null);
    setInput("");
  }, []);

  const isEmpty = messages.length === 0;

  return (
    <div
      className={`flex h-full flex-col bg-[var(--color-surface)] ${mobile ? "" : "border-l border-[var(--color-border)]"}`}
    >
      {/* Header */}
      <div className="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
        <div className="flex items-center gap-2 text-sm font-semibold text-[var(--color-fg)]">
          <Bot className="h-4 w-4" aria-hidden="true" />
          Ask AI
        </div>
        {!isEmpty && (
          <Button
            variant="ghost"
            size="icon"
            onClick={clear}
            aria-label="Clear conversation"
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        )}
      </div>

      {/* Quick-action chips — shown only when chat is empty */}
      {isEmpty && (
        <div className="flex flex-col gap-2 p-4">
          <p className="text-xs text-[var(--color-fg-muted)]">Try asking</p>
          {["Summarize this document", "What are the key takeaways?"].map((prompt) => (
            <button
              key={prompt}
              onClick={() => void send(prompt)}
              disabled={streaming}
              className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-muted)] px-3 py-2 text-left text-sm text-[var(--color-fg)] transition-opacity hover:opacity-80 disabled:opacity-50"
            >
              {prompt}
            </button>
          ))}
        </div>
      )}

      {/* Message thread */}
      {!isEmpty && (
        <div className="flex-1 overflow-y-auto space-y-3 px-4 py-3">
          {messages.map((m) => (
            <div
              key={m.id}
              className={`flex ${m.role === "user" ? "justify-end" : "justify-start"}`}
            >
              <div
                className={`max-w-[85%] rounded-xl px-3 py-2 text-sm whitespace-pre-wrap ${
                  m.role === "user"
                    ? "bg-[var(--color-accent)] text-[var(--color-accent-fg)]"
                    : "bg-[var(--color-surface-muted)] text-[var(--color-fg)]"
                }`}
              >
                {m.content}
                {m.role === "assistant" && m.content === "" && streaming && (
                  <span className="inline-block h-3 w-1 animate-pulse bg-current align-middle" />
                )}
              </div>
            </div>
          ))}
          <div ref={bottomRef} />
        </div>
      )}

      {/* Error banner */}
      {error && (
        <div className="mx-4 mb-2 rounded-lg border border-[var(--color-danger)] bg-[var(--color-accent-muted)] px-3 py-2 text-sm text-[var(--color-danger)]">
          {error}
        </div>
      )}

      {/* Input */}
      <div className="border-t border-[var(--color-border)] p-3">
        <div className="flex items-end gap-2">
          <textarea
            rows={2}
            value={input}
            disabled={streaming}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                void send(input);
              }
            }}
            placeholder="Ask anything about this document…"
            className="flex-1 resize-none rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-muted)] px-3 py-2 text-sm text-[var(--color-fg)] placeholder:text-[var(--color-fg-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--color-accent)] disabled:opacity-50"
          />
          <Button
            size="icon"
            onClick={() => void send(input)}
            disabled={streaming || !input.trim()}
            aria-label="Send"
          >
            <Send className="h-4 w-4" />
          </Button>
        </div>
        <p className="mt-1 text-xs text-[var(--color-fg-muted)]">
          Enter to send · Shift+Enter for newline
        </p>
      </div>
    </div>
  );
}
