const CARD_LINE_PATTERN = /^(\d{15,16})\|(0?[1-9]|1[0-2])\|(\d{2}|\d{4})(\|\d{3,4})?$/;

        export function normalizeToken(value = '') {
            return value.replace(/\s+/g, '').trim();
        }

        export function splitCardLines(value = '') {
            return value
                .split(/\r?\n/)
                .map((line) => line.trim())
                .filter(Boolean);
        }

        export function validateCardLine(line = '') {
            const trimmed = line.trim();
            if (!trimmed) {
                return { valid: false, message: 'Card input is required.' };
            }
            if (!CARD_LINE_PATTERN.test(trimmed)) {
                return { valid: false, message: 'Use number|month|year or number|month|year|cvv.' };
            }
            return { valid: true, value: trimmed };
        }

        export function validatePaymentPayload({ mode = 'manual', cards = [], hpp = '' }) {
            const errors = [];
            const normalizedCards = cards.map((card) => card.trim()).filter(Boolean);

            if (!normalizedCards.length) {
                errors.push('Enter at least one card in number|month|year format.');
            }
            if (mode === 'manual' && normalizedCards.length > 1) {
                errors.push('Manual mode only supports a single card per submission.');
            }
            normalizedCards.forEach((card, index) => {
                const validation = validateCardLine(card);
                if (!validation.valid) {
                    errors.push(`Card ${index + 1}: ${validation.message}`);
                }
            });
            if (!hpp.trim()) {
                errors.push('HPP content is required before submitting a payment.');
            }

            return {
                valid: errors.length === 0,
                errors,
                cards: normalizedCards,
            };
        }
