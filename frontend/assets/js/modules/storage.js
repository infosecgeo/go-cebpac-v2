import { safeJSONParse } from '../utils/helpers.js';

const STORAGE_PREFIX = 'cebpac.frontend';
const KEYS = {
    form: `${STORAGE_PREFIX}.form`,
    theme: `${STORAGE_PREFIX}.theme`,
    auth: `${STORAGE_PREFIX}.auth`,
    preferences: `${STORAGE_PREFIX}.preferences`,
};

class StorageManager {
    constructor(storage = globalThis.localStorage) {
        this.storage = storage;
    }

    isAvailable() {
        return Boolean(this.storage);
    }

    get(key, fallback = null) {
        if (!this.isAvailable()) {
            return fallback;
        }
        return this.storage.getItem(key) ?? fallback;
    }

    set(key, value) {
        if (!this.isAvailable()) {
            return;
        }
        this.storage.setItem(key, value);
    }

    getJSON(key, fallback = null) {
        return safeJSONParse(this.get(key), fallback);
    }

    setJSON(key, value) {
        this.set(key, JSON.stringify(value));
    }

    remove(key) {
        if (this.isAvailable()) {
            this.storage.removeItem(key);
        }
    }

    saveFormData(value) {
        this.setJSON(KEYS.form, value);
    }

    getFormData() {
        return this.getJSON(KEYS.form, {
            mode: 'manual',
            cardInput: '',
            bearerToken: '',
            xAuthToken: '',
            hpp: '',
        });
    }

    saveTheme(theme) {
        this.set(KEYS.theme, theme);
    }

    getTheme() {
        return this.get(KEYS.theme, 'dark');
    }

    saveAuthTokens(tokens) {
        this.setJSON(KEYS.auth, tokens);
    }

    getAuthTokens() {
        return this.getJSON(KEYS.auth, {
            accessToken: null,
            refreshToken: null,
            expiresAt: null,
        });
    }

    clearAuthTokens() {
        this.remove(KEYS.auth);
    }

    saveUserPreferences(preferences) {
        this.setJSON(KEYS.preferences, preferences);
    }

    getUserPreferences() {
        return this.getJSON(KEYS.preferences, {
            compactResults: false,
            reconnectWebSocket: true,
        });
    }
}

export const storage = new StorageManager();
export default storage;
