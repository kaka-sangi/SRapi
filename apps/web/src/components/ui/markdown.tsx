"use client";

import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

/**
 * Markdown renderer styled for the warm-paper theme. Supports GFM (tables,
 * strikethrough, task lists). Used for copilot assistant messages.
 */
export function Markdown({ children, className }: { children: string; className?: string }) {
  return (
    <div className={`space-y-2 text-sm leading-relaxed text-srapi-text-primary ${className ?? ""}`}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          p: ({ children }) => <p className="whitespace-pre-wrap">{children}</p>,
          a: ({ children, href }) => (
            <a href={href} target="_blank" rel="noreferrer" className="text-srapi-primary underline underline-offset-2">
              {children}
            </a>
          ),
          strong: ({ children }) => <strong className="font-semibold text-srapi-text-primary">{children}</strong>,
          ul: ({ children }) => <ul className="list-disc space-y-1 pl-5">{children}</ul>,
          ol: ({ children }) => <ol className="list-decimal space-y-1 pl-5">{children}</ol>,
          li: ({ children }) => <li className="marker:text-srapi-text-tertiary">{children}</li>,
          h1: ({ children }) => <h1 className="font-serif text-lg text-srapi-text-primary">{children}</h1>,
          h2: ({ children }) => <h2 className="font-serif text-base text-srapi-text-primary">{children}</h2>,
          h3: ({ children }) => <h3 className="font-serif text-sm font-semibold text-srapi-text-primary">{children}</h3>,
          blockquote: ({ children }) => (
            <blockquote className="border-l-2 border-srapi-border pl-3 text-srapi-text-secondary">{children}</blockquote>
          ),
          hr: () => <hr className="border-srapi-border" />,
          code: ({ className: cls, children }) => {
            const isBlock = /language-/.test(cls ?? "");
            if (isBlock) {
              return <code className="font-mono text-xs">{children}</code>;
            }
            return (
              <code className="rounded bg-srapi-card-muted px-1 py-0.5 font-mono text-[0.85em] text-srapi-text-primary">
                {children}
              </code>
            );
          },
          pre: ({ children }) => (
            <pre className="overflow-x-auto rounded-lg border border-srapi-border bg-srapi-card-muted/60 p-3 text-xs">
              {children}
            </pre>
          ),
          table: ({ children }) => (
            <div className="overflow-x-auto">
              <table className="w-full border-collapse text-xs">{children}</table>
            </div>
          ),
          thead: ({ children }) => <thead className="bg-srapi-card-muted/50">{children}</thead>,
          th: ({ children }) => (
            <th className="border border-srapi-border px-2 py-1 text-left font-medium text-srapi-text-secondary">
              {children}
            </th>
          ),
          td: ({ children }) => <td className="border border-srapi-border px-2 py-1 align-top">{children}</td>,
        }}
      >
        {children}
      </ReactMarkdown>
    </div>
  );
}
