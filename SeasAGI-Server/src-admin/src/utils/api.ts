export const API_BASE = "/api/v1/admin";

/** 401/403 时广播，供 AdminGate 回到登录态。 */
export const ADMIN_UNAUTHORIZED_EVENT = "seasagi:admin-unauthorized";

// 只记录「本次标签页是否登录过」的非敏感标记，用于减少挂载时的登录页闪烁。
// 真正的凭据是服务端签发的 HttpOnly Cookie，浏览器 JS 读不到，
// 因此这里绝不存放 ADMIN_SECRET 或会话令牌。
const SESSION_FLAG_KEY = "seasagi.admin.session";

export function isAuthenticated(): boolean {
  try {
    return sessionStorage.getItem(SESSION_FLAG_KEY) === "1";
  } catch {
    return false;
  }
}

function setAuthenticated(): void {
  try {
    sessionStorage.setItem(SESSION_FLAG_KEY, "1");
  } catch {
    // 隐私模式下 sessionStorage 可能不可写；Cookie 仍是唯一凭据，不影响鉴权。
  }
}

function clearAuthenticated(broadcast = true): void {
  try {
    sessionStorage.removeItem(SESSION_FLAG_KEY);
  } catch {
    // ignore
  }
  if (broadcast) {
    window.dispatchEvent(new Event(ADMIN_UNAUTHORIZED_EVENT));
  }
}

/**
 * 后台统一请求入口。会话随 Cookie 由浏览器自动携带（credentials: same-origin），
 * 前端不再注入 Authorization，也不再持有长期管理员密钥。
 */
export async function apiFetch<T = unknown>(path: string, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(init?.headers as Record<string, string> | undefined),
  };
  const res = await fetch(`${API_BASE}${path}`, {
    ...init,
    credentials: "same-origin",
    headers,
  });
  if (res.status === 401 || res.status === 403) {
    clearAuthenticated();
    throw new Error(`API ${res.status}: admin session rejected`);
  }
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`API ${res.status}: ${text}`);
  }
  return res.json();
}

/** 挂载时向服务端确认会话是否仍有效；Cookie 无效或过期时返回 false。 */
export async function verifySession(): Promise<boolean> {
  try {
    const res = await fetch(`${API_BASE}/session`, { credentials: "same-origin" });
    if (!res.ok) {
      clearAuthenticated(false);
      return false;
    }
    setAuthenticated();
    return true;
  } catch {
    return false;
  }
}

/** 用 ADMIN_SECRET 换取一次短时 HttpOnly 会话；密钥只在本次请求体中提交。 */
export async function loginAdmin(secret: string): Promise<boolean> {
  const trimmed = secret.trim();
  if (!trimmed) return false;
  try {
    const res = await fetch(`${API_BASE}/session`, {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ secret: trimmed }),
    });
    if (!res.ok) return false;
    setAuthenticated();
    return true;
  } catch {
    return false;
  }
}

/** 注销并清理本地标记。 */
export async function logoutAdmin(): Promise<void> {
  try {
    await fetch(`${API_BASE}/session`, {
      method: "DELETE",
      credentials: "same-origin",
    });
  } catch {
    // 网络失败时服务端会话仍在，仍需清理本地状态回到登录页。
  }
  // 主动注销不广播 401 事件，避免登录门误显示「登录已失效」。
  clearAuthenticated(false);
}
