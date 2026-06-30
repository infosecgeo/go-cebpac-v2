import state from '../modules/state.js';
import { formatDuration, formatLatency, maskCard } from '../utils/formatters.js';

export class ProgressTracker {
    constructor() {
        this.container = null;
        this.elements = {};
    }

    mount(container) {
        this.container = container;
        this.container.innerHTML = `
            <section class="card stack-md" aria-labelledby="progress-title">
                <div class="card__header">
                    <div>
                        <h2 class="card__title" id="progress-title">Real-time progress</h2>
                        <p class="card__subtitle">Live task state, current card context, proxy telemetry, and retry visibility.</p>
                    </div>
                </div>
                <div class="stack-sm">
                    <div class="metric-inline"><span class="metric-label">Completion</span><strong class="metric-value" id="progress-percent">0%</strong></div>
                    <div class="progress progress--compact"><div class="progress__bar" id="progress-bar"></div></div>
                </div>
                <dl class="key-value-grid">
                    <div class="key-value"><dt>Current task</dt><dd id="progress-task">Idle</dd></div>
                    <div class="key-value"><dt>Current card</dt><dd id="progress-card">—</dd></div>
                    <div class="key-value"><dt>Current proxy</dt><dd id="progress-proxy">Waiting for backend telemetry</dd></div>
                    <div class="key-value"><dt>Retry count</dt><dd id="progress-retries">0</dd></div>
                    <div class="key-value"><dt>Network latency</dt><dd id="progress-latency">—</dd></div>
                    <div class="key-value"><dt>Avg. duration</dt><dd id="progress-average">—</dd></div>
                </dl>
            </section>
        `;
        this.elements = {
            percent: this.container.querySelector('#progress-percent'),
            bar: this.container.querySelector('#progress-bar'),
            task: this.container.querySelector('#progress-task'),
            card: this.container.querySelector('#progress-card'),
            proxy: this.container.querySelector('#progress-proxy'),
            retries: this.container.querySelector('#progress-retries'),
            latency: this.container.querySelector('#progress-latency'),
            average: this.container.querySelector('#progress-average'),
        };
        this.render(state.getState());
        state.subscribe((nextState) => this.render(nextState));
    }

    render(nextState) {
        const { processing, stats, socket } = nextState;
        this.elements.percent.textContent = `${processing.progress || 0}%`;
        this.elements.bar.style.width = `${processing.progress || 0}%`;
        this.elements.task.textContent = processing.currentTask || 'Idle';
        this.elements.card.textContent = processing.currentCard ? maskCard(processing.currentCard) : '—';
        this.elements.proxy.textContent = processing.currentProxy || 'Waiting for backend telemetry';
        this.elements.retries.textContent = String(processing.retryCount);
        this.elements.latency.textContent = formatLatency(stats.latencyMs ?? socket.latencyMs);
        this.elements.average.textContent = formatDuration(processing.averageDurationMs);
    }
}
