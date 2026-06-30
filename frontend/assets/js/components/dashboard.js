import state from '../modules/state.js';
import { formatDuration, formatLatency, formatNumber, formatSystemValue } from '../utils/formatters.js';

export class Dashboard {
    constructor() {
        this.container = null;
        this.elements = {};
    }

    mount(container) {
        this.container = container;
        this.container.innerHTML = `
            <section class="card stack-md" aria-labelledby="dashboard-title">
                <div class="card__header">
                    <div>
                        <h2 class="card__title" id="dashboard-title">Operations dashboard</h2>
                        <p class="card__subtitle">Live payment throughput, queue health, infrastructure telemetry, and active session visibility.</p>
                    </div>
                    <div class="badge-row">
                        <span class="badge badge--info" id="dashboard-mode">Manual mode</span>
                        <span class="badge badge--warning" id="dashboard-status">Idle</span>
                    </div>
                </div>
                <div class="progress" aria-hidden="true"><div class="progress__bar" id="dashboard-progress"></div></div>
                <div class="stats-grid">
                    <article class="stat-card"><span class="metric-label">Current index</span><strong class="metric-value" id="metric-current-index">0 / 0</strong><span class="metric-note">Processed cards in this run</span></article>
                    <article class="stat-card"><span class="metric-label">Success / failed</span><strong class="metric-value" id="metric-results">0 / 0</strong><span class="metric-note">Live approvals versus failures</span></article>
                    <article class="stat-card"><span class="metric-label">Retries</span><strong class="metric-value" id="metric-retries">0</strong><span class="metric-note">Automatic recovery attempts</span></article>
                    <article class="stat-card"><span class="metric-label">Queue length</span><strong class="metric-value" id="metric-queue">0</strong><span class="metric-note">Cards waiting to be submitted</span></article>
                    <article class="stat-card"><span class="metric-label">Estimated time</span><strong class="metric-value" id="metric-eta">—</strong><span class="metric-note">Projection based on average runtime</span></article>
                    <article class="stat-card"><span class="metric-label">Network latency</span><strong class="metric-value" id="metric-latency">—</strong><span class="metric-note">Client-to-telemetry round trip</span></article>
                    <article class="stat-card"><span class="metric-label">Memory usage</span><strong class="metric-value" id="metric-memory">—</strong><span class="metric-note">Backend runtime memory footprint</span></article>
                    <article class="stat-card"><span class="metric-label">CPU usage</span><strong class="metric-value" id="metric-cpu">—</strong><span class="metric-note">Backend CPU utilization</span></article>
                    <article class="stat-card"><span class="metric-label">Active users</span><strong class="metric-value" id="metric-users">0</strong><span class="metric-note">Connected operators</span></article>
                    <article class="stat-card"><span class="metric-label">Active sessions</span><strong class="metric-value" id="metric-sessions">0</strong><span class="metric-note">Open payment sessions</span></article>
                </div>
            </section>
        `;

        this.elements = {
            mode: this.container.querySelector('#dashboard-mode'),
            status: this.container.querySelector('#dashboard-status'),
            progressBar: this.container.querySelector('#dashboard-progress'),
            currentIndex: this.container.querySelector('#metric-current-index'),
            results: this.container.querySelector('#metric-results'),
            retries: this.container.querySelector('#metric-retries'),
            queue: this.container.querySelector('#metric-queue'),
            eta: this.container.querySelector('#metric-eta'),
            latency: this.container.querySelector('#metric-latency'),
            memory: this.container.querySelector('#metric-memory'),
            cpu: this.container.querySelector('#metric-cpu'),
            users: this.container.querySelector('#metric-users'),
            sessions: this.container.querySelector('#metric-sessions'),
        };

        this.render(state.getState());
        state.subscribe((nextState) => this.render(nextState));
    }

    render(nextState) {
        const { processing, stats, user } = nextState;
        this.elements.mode.textContent = `${processing.mode === 'auto' ? 'Automatic' : 'Manual'} mode`;
        this.elements.status.textContent = processing.running ? 'Processing' : 'Idle';
        this.elements.status.className = `badge ${processing.running ? 'badge--info' : 'badge--warning'}`;
        this.elements.progressBar.style.width = `${processing.progress || 0}%`;
        this.elements.currentIndex.textContent = `${formatNumber(processing.currentIndex)} / ${formatNumber(processing.total)}`;
        this.elements.results.textContent = `${formatNumber(processing.successCount)} / ${formatNumber(processing.failedCount)}`;
        this.elements.retries.textContent = formatNumber(processing.retryCount);
        this.elements.queue.textContent = formatNumber(processing.queueLength);
        this.elements.eta.textContent = formatDuration(processing.estimatedMs);
        this.elements.latency.textContent = formatLatency(stats.latencyMs ?? nextState.socket.latencyMs);
        this.elements.memory.textContent = formatSystemValue(stats.memoryUsageMb, ' MB');
        this.elements.cpu.textContent = formatSystemValue(stats.cpuPercent, '%');
        this.elements.users.textContent = formatNumber(user.activeUsers);
        this.elements.sessions.textContent = formatNumber(user.activeSessions);
    }
}
