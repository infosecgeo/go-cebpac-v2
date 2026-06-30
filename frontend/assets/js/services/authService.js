import api from '../modules/api.js';
import storage from '../modules/storage.js';
import state from '../modules/state.js';

class AuthService {
    async login(credentials) {
        const payload = await api.post('/auth/login', credentials, { auth: false });
        const tokens = {
            accessToken: payload.accessToken,
            refreshToken: payload.refreshToken,
            expiresAt: payload.expiresAt ?? null,
        };
        storage.saveAuthTokens(tokens);
        state.setState({
            auth: {
                ...tokens,
                authenticated: true,
            },
            user: {
                profile: payload.user ?? null,
            },
        }, 'auth:login');
        return payload;
    }

    async logout() {
        try {
            await api.post('/auth/logout', {}, { retryOnAuth: false });
        } catch {
            // Logout should always clear the local session.
        }
        storage.clearAuthTokens();
        state.setState({
            auth: {
                accessToken: null,
                refreshToken: null,
                expiresAt: null,
                authenticated: false,
            },
            user: {
                profile: null,
            },
        }, 'auth:logout');
    }

    refresh() {
        return api.refreshToken();
    }
}

export const authService = new AuthService();
export default authService;
