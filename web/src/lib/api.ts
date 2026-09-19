import { saveTokens, TOKENS, clearTokens } from "./auth";
import { API_BASE_URL } from "./information";

export const isAuthenticated = (): boolean => {
  for (const key of TOKENS) {
    const val = localStorage.getItem(key);
    if (!val) return false;
  }
  return true;
};

export const Logout = async () => {
  try {
    await Call("/auth/logout", "POST")
  } finally {
    deleteTokens();
  }
}

export const LogoutAll = async () => {
  try {
    await Call("/auth/logout/all", "POST")
  } finally {
    deleteTokens();
  }
}

export const deleteTokens = () => {
  clearTokens();
};

export const isTokenExpired = () => {
  const expStr = localStorage.getItem('access_token_expires_at');
  if (!expStr) return true;
  return new Date(expStr) < new Date();
};

export const refreshToken = async () => {
  const expStr = localStorage.getItem('refresh_token_expires_at');
  if (!expStr) throw new UnauthorizedError("refresh_token_expires_at not found");
  if (new Date(expStr) < new Date()) {
    deleteTokens();
    throw new UnauthorizedError("Refresh token expired");
  }
  const token = localStorage.getItem('refresh_token');

  const resp = await fetch(`${API_BASE_URL}/auth/refresh`, {
    method: "POST",
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      refresh_token: token
    })
  })
  if (!resp.ok) {
    const { error } = await resp.json()
    if (resp.status === 400) {
      throw new UnauthorizedError(error)
    } else {
      throw new Error(error || "Request Failed")
    }
  } else {
    const data = await resp.json()
    console.log('Refreshed tokens', data);
    saveTokens(data)
  }
}


export class UnauthorizedError extends Error { }

/** What the API answers a failure with: a title, a sentence written for the
 *  person reading it, a stable machine-readable code, and the request id an
 *  operator can find the server-side log line by. */
export interface APIErrorBody {
  error?: string;
  message: string;
  code?: string;
  request_id?: string;
}

export class APIError<T extends APIErrorBody = APIErrorBody> extends Error {
  status: number;
  /** Always an object with a non-empty `message`, so a caller rendering
   *  `body.message` as the detail line never shows an empty one. A proxy 502,
   *  an HTML error page and a dropped connection all answer with no JSON at
   *  all, and each of those used to reach the user as a blank explanation
   *  under a bare "Internal Server Error". */
  body: T;
  /** Stable condition identifier, for branching rather than display. */
  code?: string;
  requestId?: string;
  constructor(message: string, status: number, body: T) {
    super(message);
    this.status = status;
    this.body = body;
    this.code = body.code;
    this.requestId = body.request_id;
  }
}

/** The sentence to show when the response carried none of its own. Each names
 *  what the person can do about it rather than restating the status code. */
function fallbackMessage(status: number): string {
  if (status === 0) return "We couldn't reach the server. Check your connection and try again.";
  if (status === 401) return "Your session has expired. Sign in again to continue.";
  if (status === 403) return "You don't have permission to do that.";
  if (status === 404) return "That item no longer exists. It may have been deleted.";
  if (status === 409) return "Someone else changed this first. Reload the page and try again.";
  if (status === 413) return "That file is too large to upload.";
  if (status === 429) return "You're going a little fast. Wait a moment and try again.";
  if (status === 503) return "The service is temporarily unavailable. Try again in a moment.";
  if (status >= 500) return "Something went wrong on our end. Nothing was changed. Try again in a moment.";
  return "The request couldn't be completed. Check the details and try again.";
}

/** Reads the API's error envelope off a response, whatever it turned out to
 *  be. A body that is not JSON (a proxy's HTML error page) or that is JSON
 *  without our envelope both end up with a usable message rather than
 *  `undefined`. */
async function errorBody(res: Response): Promise<APIErrorBody> {
  const raw: unknown = await res.json().catch(() => null);
  const parsed = (raw && typeof raw === "object" ? raw : {}) as Partial<APIErrorBody>;
  return {
    ...parsed,
    error: parsed.error || res.statusText || "Request failed",
    message: parsed.message || fallbackMessage(res.status),
  };
}

type FetchMethod = 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH';

export async function Call(
  endpoint: string,
  method: FetchMethod = 'GET',
  body?: object,
  nocontent = false,
  // retried is set by the one 401 retry below. A second 401 after a successful
  // refresh means the token is not the problem, and recursing on it spun a tab
  // through the endpoint until the user closed it.
  retried = false,
) {
  if (isTokenExpired()) {
    await refreshToken();
  }

  const token = localStorage.getItem('access_token');

  let res: Response;
  try {
    res = await fetch(`${API_BASE_URL}${endpoint}`, {
      method,
      headers: {
        'Content-Type': 'application/json',
        ...(token && { Authorization: `Bearer ${token}` }),
      },
      ...(body && { body: JSON.stringify(body) }),
    });
  } catch {
    // fetch only rejects when the request never got an answer: offline, DNS,
    // TLS, CORS, or a cancelled navigation. It reached the user as the raw
    // "TypeError: Failed to fetch", which reads as a bug in the app rather
    // than as a connection that dropped.
    throw new APIError('Network Error', 0, {
      error: 'Network Error',
      message: fallbackMessage(0),
    });
  }

  if (!res.ok) {
    if (res.status === 401 && !retried) {
      await refreshToken();
      return await Call(endpoint, method, body, nocontent, true);
    }

    const msg = await errorBody(res);
    throw new APIError(msg.error || 'Request failed', res.status, msg);
  }
  if (!nocontent) {
    return await res.json();
  }
  return
}
