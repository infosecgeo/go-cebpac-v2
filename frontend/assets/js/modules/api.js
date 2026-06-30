import storage from './storage.js';
import state from './state.js';
import { formatErrorMessage } from '../utils/helpers.js';

class ApiClient extends EventTarget {
    constructor({ baseUrl = '' } = {}) {
        super();
        this.baseUrl = baseUrl;
        this.refreshPromise = null;
    }

    buildHeaders(headers = {}, body, auth = true) {
        const requestHeaders = new Headers(headers);
        if (auth) {
            const tokens = storage.getAuthTokens();
            if (tokens.accessToken && !requestHeaders.has('Authorization')) {
                requestHeaders.set('Authorization', ['Bearer', tokens.accessToken].join(' '));
            }
        }
        if (!(body instanceof FormData) && body !== undefined && body !== null && !requestHeaders.has('Content-Type')) {
            requestHeaders.set('Content-Type', 'application/json');
        }
        if (!requestHeaders.has('Accept')) {
            requestHeaders.set('Accept', 'application/json');
        }
        return requestHeaders;
    }

    async request(path, options = {}) {
        const {
            auth = true,
            retryOnAuth = true,
            timeout = 30000,
            ...requestOptions
        } = options;
        const url = path.startsWith('http') ? path : `${this.baseUrl}${path}`;
        const controller = new AbortController();
        const timeoutId = window.setTimeout(() => controller.abort(new Error('Request timed out.')), timeout);

        state.updateLoading(1);
        this.dispatchEvent(new CustomEvent('loading', { detail: { active: true, url } }));

        const makeRequest = async () => fetch(url, {
            credentials: 'same-origin',
            ...requestOptions,
            headers: this.buildHeaders(requestOptions.headers, requestOptions.body, auth),
            signal: controller.signal,
        });

        try {
            let response = await makeRequest();
            if (response.status === 401 && auth && retryOnAuth) {
                const refreshed = await this.refreshToken();
                if (refreshed) {
                    response = await makeRequest();
                }
            }
            return await this.parseResponse(response);
        } catch (error) {
            const normalizedError = new Error(formatErrorMessage(error));
            normalizedError.cause = error;
            throw normalizedError;
        } finally {
            window.clearTimeout(timeoutId);
            state.updateLoading(-1);
            this.dispatchEvent(new CustomEvent('loading', { detail: { active: false, url } }));
        }
    }

    async parseResponse(response) {
        const contentType = response.headers.get('content-type') || '';
        const payload = contentType.includes('application/json')
            ? await response.json().catch(() => null)
            : await response.text().catch(() => '');

        if (!response.ok) {
            const error = new Error(payload?.message || payload || response.statusText || 'Request failed.');
            error.status = response.status;
            error.payload = payload;
            throw error;
        }

        return payload;
    }

    get(path, options = {}) {
        return this.request(path, { ...options, method: 'GET' });
    }

    post(path, data, options = {}) {
        return this.request(path, {
            ...options,
            method: 'POST',
            body: JSON.stringify(data ?? {}),
        });
    }

    postForm(path, data, options = {}) {
        const formData = data instanceof FormData ? data : Object.entries(data ?? {}).reduce((form, [key, value]) => {
            if (value !== undefined && value !== null) {
                form.append(key, value);
            }
            return form;
        }, new FormData());
        return this.request(path, {
            ...options,
            method: 'POST',
            body: formData,
        });
    }

    async refreshToken() {
        if (this.refreshPromise) {
            return this.refreshPromise;
        }

        const tokens = storage.getAuthTokens();
        if (!tokens.refreshToken) {
            return false;
        }

        state.setState({ auth: { refreshing: true } }, 'auth:refresh:start');
        this.refreshPromise = fetch(`${this.baseUrl}/auth/refresh`, {
            method: 'POST',
            credentials: 'same-origin',
            headers: new Headers({
                'Content-Type': 'application/json',
                Accept: 'application/json',
            }),
            body: JSON.stringify({ refreshToken: tokens.refreshToken }),
        })
            .then(async (response) => {
                if (!response.ok) {
                    throw new Error('Your session expired. Please sign in again.');
                }
                const payload = await response.json();
                const nextTokens = {
                    accessToken: payload.accessToken,
                    refreshToken: payload.refreshToken ?? tokens.refreshToken,
                    expiresAt: payload.expiresAt ?? null,
                };
                storage.saveAuthTokens(nextTokens);
                state.setState({
                    auth: {
                        ...nextTokens,
                        authenticated: Boolean(nextTokens.accessToken),
                        refreshing: false,
                    },
                }, 'auth:refresh:success');
                return true;
            })
            .catch(() => {
                storage.clearAuthTokens();
                state.setState({
                    auth: {
                        accessToken: null,
                        refreshToken: null,
                        expiresAt: null,
                        authenticated: false,
                        refreshing: false,
                    },
                }, 'auth:refresh:failed');
                return false;
            })
            .finally(() => {
                this.refreshPromise = null;
            });

        return this.refreshPromise;
    }
}

export const api = new ApiClient();
export default api;
