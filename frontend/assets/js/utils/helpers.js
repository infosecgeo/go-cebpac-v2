export function safeJSONParse(value, fallback = null) {
            try {
                return value ? JSON.parse(value) : fallback;
            } catch {
                return fallback;
            }
        }

        export function clone(value) {
            if (typeof globalThis.structuredClone === 'function') {
                return globalThis.structuredClone(value);
            }
            return JSON.parse(JSON.stringify(value));
        }

        export function debounce(callback, wait = 250) {
            let timeoutId;
            return (...args) => {
                clearTimeout(timeoutId);
                timeoutId = window.setTimeout(() => callback(...args), wait);
            };
        }

        export function clamp(value, min, max) {
            return Math.min(Math.max(value, min), max);
        }

        export function uniqueId(prefix = 'id') {
            if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
                return `${prefix}-${crypto.randomUUID()}`;
            }
            return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`;
        }

        export function createElement(tag, { className, text, html, attrs = {}, dataset = {}, children = [] } = {}) {
            const element = document.createElement(tag);
            if (className) {
                element.className = className;
            }
            if (text !== undefined) {
                element.textContent = text;
            }
            if (html !== undefined) {
                element.innerHTML = html;
            }
            Object.entries(attrs).forEach(([key, value]) => {
                if (value !== undefined && value !== null) {
                    element.setAttribute(key, String(value));
                }
            });
            Object.entries(dataset).forEach(([key, value]) => {
                if (value !== undefined && value !== null) {
                    element.dataset[key] = String(value);
                }
            });
            children.filter(Boolean).forEach((child) => element.appendChild(child));
            return element;
        }

        export async function copyToClipboard(text) {
            if (navigator.clipboard?.writeText) {
                await navigator.clipboard.writeText(text);
                return;
            }

            const textArea = document.createElement('textarea');
            textArea.value = text;
            textArea.setAttribute('readonly', 'readonly');
            textArea.style.position = 'fixed';
            textArea.style.opacity = '0';
            document.body.appendChild(textArea);
            textArea.select();
            document.execCommand('copy');
            document.body.removeChild(textArea);
        }

        export function downloadTextFile(filename, content, type = 'text/plain;charset=utf-8') {
            const blob = new Blob([content], { type });
            const url = URL.createObjectURL(blob);
            const link = document.createElement('a');
            link.href = url;
            link.download = filename;
            document.body.appendChild(link);
            link.click();
            link.remove();
            URL.revokeObjectURL(url);
        }

        export function formatErrorMessage(error) {
            if (!error) {
                return 'Something went wrong.';
            }
            if (typeof error === 'string') {
                return error;
            }
            return error.message || error.payload?.message || 'Something went wrong.';
        }

        export function toCsv(rows) {
            return rows
                .map((row) => row.map((cell) => `"${String(cell ?? '').replaceAll('"', '""')}"`).join(','))
                .join('\n');
        }
