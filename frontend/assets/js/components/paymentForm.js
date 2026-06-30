import storage from '../modules/storage.js';
import state from '../modules/state.js';
import paymentService from '../services/paymentService.js';
import { debounce } from '../utils/helpers.js';
import { normalizeToken, splitCardLines } from '../utils/validators.js';

export class PaymentForm {
    constructor({ toast, modal }) {
        this.toast = toast;
        this.modal = modal;
        this.container = null;
        this.form = null;
        this.fields = {};
        this.modeButtons = {};
        this.persistForm = debounce(() => this.saveState(), 150);
    }

    mount(container) {
        this.container = container;
        this.container.innerHTML = `
            <section class="card stack-md" aria-labelledby="payment-form-title">
                <div class="card__header">
                    <div>
                        <h2 class="card__title" id="payment-form-title">Payment execution</h2>
                        <p class="card__subtitle">Queue cards for manual or automatic processing with persistent local form recovery.</p>
                    </div>
                    <div class="badge-row"><span class="badge badge--info" id="card-count-badge">0 cards queued</span></div>
                </div>
                <form id="payment-form" class="field-grid" novalidate>
                    <div class="field">
                        <span class="field__label-row"><label id="mode-label">Processing mode</label><span class="field__hint">Auto mode submits cards sequentially.</span></span>
                        <div class="mode-switch" role="tablist" aria-labelledby="mode-label">
                            <button type="button" class="mode-switch__button is-active" id="mode-manual" data-mode="manual" role="tab" aria-selected="true">Manual</button>
                            <button type="button" class="mode-switch__button" id="mode-auto" data-mode="auto" role="tab" aria-selected="false">Automatic</button>
                        </div>
                    </div>
                    <div class="field" id="card-field">
                        <div class="field__label-row"><label for="card-input">Card input</label><span class="field__hint" id="card-helper">Use one card: number|month|year|cvv(optional)</span></div>
                        <textarea class="textarea textarea--cards input--mono" id="card-input" name="cardInput" aria-describedby="card-helper"></textarea>
                        <span class="field__error" id="card-error" hidden></span>
                    </div>
                    <div class="field">
                        <div class="field__label-row"><label for="hpp-input">HPP content</label><span class="field__hint">Required by the backend payment flow.</span></div>
                        <textarea class="textarea textarea--mono" id="hpp-input" name="hpp" spellcheck="false"></textarea>
                    </div>
                    <div class="field">
                        <div class="field__label-row"><label for="bearer-input">Authorization token</label><span class="field__hint">Stored locally only.</span></div>
                        <textarea class="textarea textarea--mono" id="bearer-input" name="bearerToken" spellcheck="false"></textarea>
                    </div>
                    <div class="field">
                        <div class="field__label-row"><label for="xauth-input">X-Auth-Token</label><span class="field__hint">Stored locally only.</span></div>
                        <textarea class="textarea textarea--mono" id="xauth-input" name="xAuthToken" spellcheck="false"></textarea>
                    </div>
                    <div class="btn-row">
                        <button type="submit" class="btn btn-primary" id="submit-button">Start payment run</button>
                        <button type="button" class="btn btn-secondary" id="restore-button">Restore saved</button>
                        <button type="button" class="btn btn-ghost" id="reset-button">Reset form</button>
                    </div>
                </form>
            </section>
        `;
        this.form = this.container.querySelector('#payment-form');
        this.fields = {
            cardInput: this.container.querySelector('#card-input'),
            hpp: this.container.querySelector('#hpp-input'),
            bearerToken: this.container.querySelector('#bearer-input'),
            xAuthToken: this.container.querySelector('#xauth-input'),
            cardError: this.container.querySelector('#card-error'),
            cardHelper: this.container.querySelector('#card-helper'),
            cardCount: this.container.querySelector('#card-count-badge'),
            submit: this.container.querySelector('#submit-button'),
        };
        this.modeButtons = {
            manual: this.container.querySelector('#mode-manual'),
            auto: this.container.querySelector('#mode-auto'),
        };

        this.attachEvents();
        this.restoreForm();
        this.render(state.getState());
        state.subscribe((nextState) => this.render(nextState), (nextState) => nextState.processing.running);
    }

    attachEvents() {
        Object.entries(this.modeButtons).forEach(([mode, button]) => {
            button.addEventListener('click', () => this.setMode(mode));
        });

        ['cardInput', 'hpp'].forEach((key) => {
            this.fields[key].addEventListener('input', () => {
                this.updateCardCount();
                this.persistForm();
            });
        });

        ['bearerToken', 'xAuthToken'].forEach((key) => {
            this.fields[key].addEventListener('input', () => {
                this.fields[key].value = normalizeToken(this.fields[key].value);
                this.persistForm();
            });
        });

        this.form.addEventListener('submit', (event) => this.handleSubmit(event));
        this.container.querySelector('#restore-button').addEventListener('click', () => {
            this.restoreForm();
            this.toast.show({ title: 'Saved values restored', message: 'The form has been restored from local storage.', type: 'info' });
        });
        this.container.querySelector('#reset-button').addEventListener('click', async () => {
            const confirmed = await this.modal.confirm({
                title: 'Reset saved form?',
                description: 'This clears the current form values and the saved local storage draft.',
                confirmText: 'Reset form',
            });
            if (!confirmed) {
                return;
            }
            storage.saveFormData({ mode: 'manual', cardInput: '', bearerToken: '', xAuthToken: '', hpp: '' });
            this.restoreForm();
            this.toast.show({ title: 'Form reset', message: 'Saved form values were cleared.', type: 'info' });
        });
    }

    getValues() {
        return {
            mode: this.currentMode || 'manual',
            cardInput: this.fields.cardInput.value,
            cards: splitCardLines(this.fields.cardInput.value),
            bearerToken: normalizeToken(this.fields.bearerToken.value),
            xAuthToken: normalizeToken(this.fields.xAuthToken.value),
            hpp: this.fields.hpp.value.trim(),
        };
    }

    saveState() {
        const values = this.getValues();
        storage.saveFormData({
            mode: values.mode,
            cardInput: values.cardInput,
            bearerToken: values.bearerToken,
            xAuthToken: values.xAuthToken,
            hpp: values.hpp,
        });
    }

    restoreForm() {
        const saved = storage.getFormData();
        this.fields.cardInput.value = saved.cardInput || '';
        this.fields.hpp.value = saved.hpp || '';
        this.fields.bearerToken.value = saved.bearerToken || '';
        this.fields.xAuthToken.value = saved.xAuthToken || '';
        this.setMode(saved.mode || 'manual');
        this.updateCardCount();
        this.clearErrors();
    }

    setMode(mode) {
        this.currentMode = mode;
        Object.entries(this.modeButtons).forEach(([key, button]) => {
            const active = key === mode;
            button.classList.toggle('is-active', active);
            button.setAttribute('aria-selected', String(active));
        });
        this.fields.cardHelper.textContent = mode === 'auto'
            ? 'Paste one card per line for sequential backend submission.'
            : 'Use one card: number|month|year|cvv(optional)';
        this.updateCardCount();
        this.persistForm();
    }

    updateCardCount() {
        const count = splitCardLines(this.fields.cardInput.value).length;
        this.fields.cardCount.textContent = `${count} card${count === 1 ? '' : 's'} queued`;
    }

    clearErrors() {
        this.container.querySelector('#card-field').classList.remove('field--error');
        this.fields.cardError.hidden = true;
        this.fields.cardError.textContent = '';
    }

    showCardError(message) {
        this.container.querySelector('#card-field').classList.add('field--error');
        this.fields.cardError.hidden = false;
        this.fields.cardError.textContent = message;
    }

    async handleSubmit(event) {
        event.preventDefault();
        this.clearErrors();
        const values = this.getValues();
        try {
            const results = await paymentService.submit(values);
            this.saveState();
            const liveCount = results.filter((result) => result.status === 'live').length;
            const deadCount = results.length - liveCount;
            this.toast.show({
                title: values.mode === 'auto' ? 'Batch completed' : 'Payment completed',
                message: `${liveCount} live / ${deadCount} dead result(s).`,
                type: liveCount ? 'success' : 'warning',
            });
        } catch (error) {
            this.showCardError(error.message || 'Please review the card input.');
            this.toast.show({ title: 'Unable to start payment', message: error.message || 'Review the form fields and try again.', type: 'error' });
        }
    }

    render(isRunning) {
        const running = typeof isRunning === 'boolean' ? isRunning : Boolean(isRunning?.processing?.running);
        this.fields.submit.disabled = running;
        this.fields.submit.textContent = running ? 'Processing…' : 'Start payment run';
    }
}
