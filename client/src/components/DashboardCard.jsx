export function DashboardCard({ title, description }) {
    return (
        <article className="dashboard-card">
            <h2>{title}</h2>
            <p>{description}</p>
            <div className="empty-state">Not connected</div>
        </article>
    );
}
