export class ModalManager {
    constructor() {
        this.root = null;
        this.activeResolver = null;
        this.handleKeydown = this.handleKeydown.bind(this);
    }

    mount(container) {
        this.root = container;
        this.root.className = 'modal';
        this.root.setAttribute('role', 'presentation');
        this.root.innerHTML = `
            <div class="modal__dialog" role="dialog" aria-modal="true" aria-labelledby="modal-title" aria-describedby="modal-description">
                <h2 class="modal__title" id="modal-title"></h2>
                <p class="modal__description" id="modal-description"></p>
                <div class="modal__body" id="modal-body"></div>
                <div class="modal__actions">
                    <button type="button" class="btn btn-ghost" data-action="cancel">Cancel</button>
                    <button type="button" class="btn btn-primary" data-action="confirm">Confirm</button>
                </div>
            </div>
        `;
        this.root.addEventListener('click', (event) => {
            if (event.target === this.root) {
                this.close(false);
            }
        });
        this.root.querySelector('[data-action="cancel"]').addEventListener('click', () => this.close(false));
        this.root.querySelector('[data-action="confirm"]').addEventListener('click', () => this.close(true));
    }

    handleKeydown(event) {
        if (event.key === 'Escape') {
            this.close(false);
        }
    }

    open({ title, description = '', body = '', confirmText = 'Confirm', cancelText = 'Cancel', showCancel = true }) {
        if (!this.root) {
            return Promise.resolve(false);
        }
        this.root.querySelector('#modal-title').textContent = title;
        this.root.querySelector('#modal-description').textContent = description;
        const bodyElement = this.root.querySelector('#modal-body');
        bodyElement.innerHTML = '';
        if (typeof body === 'string') {
            bodyElement.textContent = body;
        } else if (body instanceof Node) {
            bodyElement.appendChild(body);
        }
        const cancelButton = this.root.querySelector('[data-action="cancel"]');
        const confirmButton = this.root.querySelector('[data-action="confirm"]');
        cancelButton.textContent = cancelText;
        confirmButton.textContent = confirmText;
        cancelButton.hidden = !showCancel;

        this.root.classList.add('is-open');
        document.addEventListener('keydown', this.handleKeydown);
        confirmButton.focus();

        return new Promise((resolve) => {
            this.activeResolver = resolve;
        });
    }

    close(result) {
        if (!this.activeResolver || !this.root) {
            return;
        }
        const resolve = this.activeResolver;
        this.activeResolver = null;
        this.root.classList.remove('is-open');
        document.removeEventListener('keydown', this.handleKeydown);
        resolve(result);
    }

    confirm(options) {
        return this.open(options);
    }

    info(options) {
        return this.open({ ...options, confirmText: options.confirmText || 'Close', showCancel: false });
    }
}
