import api from '../modules/api.js';
import state from '../modules/state.js';
import { formatErrorMessage } from '../utils/helpers.js';
import { validatePaymentPayload } from '../utils/validators.js';

class PaymentService {
    constructor() {
        this.maxAttempts = 2;
    }

    shouldRetry(error) {
        return !error?.status || error.status >= 500;
    }

    buildPayload(card, payload) {
        return {
            mode: payload.mode,
            card,
            bearerToken: payload.bearerToken,
            xAuthToken: payload.xAuthToken,
            hpp: payload.hpp,
        };
    }

    async processCard(card, payload, attempt = 1) {
        const startedAt = performance.now();
        try {
            const response = await api.postForm('/pay', this.buildPayload(card, payload), {
                auth: false,
                retryOnAuth: false,
            });
            const result = {
                card,
                status: response?.success ? 'live' : 'dead',
                success: Boolean(response?.success),
                message: response?.message || (response?.success ? 'Payment approved.' : 'Payment failed.'),
                durationMs: performance.now() - startedAt,
                attempt,
                timestamp: new Date().toISOString(),
            };
            state.addResult(result);
            return result;
        } catch (error) {
            if (attempt < this.maxAttempts && this.shouldRetry(error)) {
                state.incrementRetry();
                state.updateProcessing({ currentTask: `Retrying request for card ${attempt + 1}` }, 'processing:retrying');
                return this.processCard(card, payload, attempt + 1);
            }
            const result = {
                card,
                status: 'dead',
                success: false,
                message: formatErrorMessage(error),
                durationMs: performance.now() - startedAt,
                attempt,
                timestamp: new Date().toISOString(),
            };
            state.addResult(result);
            return result;
        }
    }

    async submit(payload) {
        const validation = validatePaymentPayload(payload);
        if (!validation.valid) {
            throw new Error(validation.errors[0]);
        }

        const normalizedPayload = {
            ...payload,
            cards: validation.cards,
        };

        state.resetProcessing({
            mode: normalizedPayload.mode,
            total: normalizedPayload.cards.length,
        });

        const results = [];
        try {
            for (const [index, card] of normalizedPayload.cards.entries()) {
                state.updateProcessing({
                    currentTask: `Processing card ${index + 1} of ${normalizedPayload.cards.length}`,
                    currentCard: card,
                });
                const result = await this.processCard(card, normalizedPayload);
                results.push(result);
            }
        } finally {
            state.completeProcessing();
        }
        return results;
    }
}

export const paymentService = new PaymentService();
export default paymentService;
