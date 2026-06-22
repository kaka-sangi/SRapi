"use client";

import { Component, type ErrorInfo, type ReactNode } from "react";

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
}

interface State {
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { error: null };
  }

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("[ErrorBoundary]", error, info.componentStack);
  }

  render() {
    if (this.state.error) {
      if (this.props.fallback) return this.props.fallback;
      return (
        <div className="flex min-h-[200px] flex-col items-center justify-center gap-3 rounded-xl border border-srapi-error/30 bg-srapi-error/5 p-6 text-center">
          <p className="text-sm font-medium text-srapi-error">Something went wrong</p>
          <p className="max-w-md text-xs text-srapi-text-tertiary">
            {this.state.error.message}
          </p>
          <button
            type="button"
            onClick={() => this.setState({ error: null })}
            className="mt-1 rounded-lg bg-srapi-error/10 px-3 py-1.5 text-xs font-medium text-srapi-error transition-colors hover:bg-srapi-error/20"
          >
            Try again
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}
