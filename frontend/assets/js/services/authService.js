import api from '../modules/api.js';
import storage from '../modules/storage.js';
import state from '../modules/state.js';

class AuthService {
    async login(licenseKey, deviceId = null) {
        const payload = await api.post('/api/v1/auth/login', {
            license_key: licenseKey,
            device_id: deviceId || this.getDeviceId(),
        }, { auth: false });
        
        const tokens = {
            accessToken: payload.token,
            refreshToken: payload.refresh_token,
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

    async adminLogin(username, password) {
        const payload = await api.post('/api/v1/auth/admin/login', {
            username,
            password,
        }, { auth: false });
        
        const tokens = {
            accessToken: payload.token,
            refreshToken: payload.refresh_token,
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
        }, 'auth:admin_login');
        return payload;
    }

    async register(licenseKey, termsAccepted, deviceId = null) {
        const payload = await api.post('/api/v1/auth/register', {
            license_key: licenseKey,
            terms_accepted: termsAccepted,
            device_id: deviceId || this.getDeviceId(),
        }, { auth: false });
        return payload;
    }

    async logout() {
        try {
            await api.post('/api/v1/auth/logout', {}, { retryOnAuth: false });
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

    getDeviceId() {
        let deviceId = storage.getItem('device_id');
        if (!deviceId) {
            deviceId = 'device_' + Date.now() + '_' + Math.random().toString(36).substring(2, 15);
            storage.setItem('device_id', deviceId);
        }
        return deviceId;
    }

    isAuthenticated() {
        const tokens = storage.getAuthTokens();
        return Boolean(tokens.accessToken);
    }
}

export const authService = new AuthService();
export default authService;
