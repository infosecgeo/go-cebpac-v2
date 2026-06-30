import { uniqueId } from '../utils/helpers.js';

export class ToastManager {
    constructor() {
        this.root = null;
        this.toasts = new Map();
    }

    mount(container) {
        this.root = container;
        this.root.className = 'toast-stack';
    }

    show({ title, message, type = 'info', duration = 4000 }) {
        if (!this.root) {
            return null;
        }
        const id = uniqueId('toast');
        const toast = document.createElement('button');
        toast.type = 'button';
        toast.className = `toast toast--${type}`;
        toast.setAttribute('aria-label', `${type} notification. Click to dismiss.`);

        const titleElement = document.createElement('span');
        titleElement.className = 'toast__title';
        titleElement.textContent = title;

        const messageElement = document.createElement('span');
        messageElement.className = 'toast__message';
        messageElement.textContent = message;

        toast.append(titleElement, messageElement);
        toast.addEventListener('click', () => this.dismiss(id));
        this.root.appendChild(toast);
        const timer = window.setTimeout(() => this.dismiss(id), duration);
        this.toasts.set(id, { element: toast, timer });
        return id;
    }

    dismiss(id) {
        const entry = this.toasts.get(id);
        if (!entry) {
            return;
        }
        window.clearTimeout(entry.timer);
        entry.element.remove();
        this.toasts.delete(id);
    }
}
