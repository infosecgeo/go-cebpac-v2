import storage from '../modules/storage.js';
import state from '../modules/state.js';

export class ThemeToggle {
    constructor() {
        this.button = null;
    }

    applyTheme(theme) {
        document.documentElement.dataset.theme = theme;
        storage.saveTheme(theme);
        state.setState({ ui: { theme } }, 'theme:apply');
        if (this.button) {
            this.button.setAttribute('aria-pressed', String(theme === 'light'));
            this.button.textContent = theme === 'light' ? 'Use dark mode' : 'Use light mode';
        }
    }

    mount(container) {
        this.button = document.createElement('button');
        this.button.type = 'button';
        this.button.className = 'btn btn-secondary btn-icon theme-toggle';
        this.button.setAttribute('aria-label', 'Toggle dark or light theme');
        this.button.addEventListener('click', () => {
            const nextTheme = document.documentElement.dataset.theme === 'light' ? 'dark' : 'light';
            this.applyTheme(nextTheme);
        });
        container.appendChild(this.button);
        this.applyTheme(storage.getTheme());
    }
}
