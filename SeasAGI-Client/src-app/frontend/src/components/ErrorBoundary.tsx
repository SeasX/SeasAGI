import { Component, type ErrorInfo, type ReactNode } from "react";

interface Props {
  children: ReactNode;
}

interface State {
  error: Error | null;
}

// 捕获子树渲染期异常，显示可操作的错误界面，避免整个应用白屏。
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("[ErrorBoundary]", error, info.componentStack);
  }

  render() {
    if (this.state.error) {
      return (
        <div style={{ padding: 40, fontFamily: "monospace", color: "#c0392b" }}>
          <h2>页面出现异常</h2>
          <pre style={{ whiteSpace: "pre-wrap", wordBreak: "break-all", margin: "12px 0" }}>
            {this.state.error.message}
          </pre>
          <button
            type="button"
            className="btn-primary"
            onClick={() => window.location.reload()}
          >
            重新加载
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}
