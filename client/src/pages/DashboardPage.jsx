import { DashboardCard } from "../components/DashboardCard.jsx";

const dashboardCards = [{
    title: "Replay library",
    description: "Uploaded replays will appear here after account access is connected."
}, {
    title: "Opponent notes",
    description: "Clan profiles and scouting notes will live here."
}, {
    title: "Prep plans",
    description: "Saved match plans and caller notes will be shown here."
}, {
    title: "Account settings",
    description: "Profile and access controls will be available after login is implemented."
}];

export function DashboardPage() {
    return (
        <main className="dashboard-shell">
            <header className="dashboard-header">
                <div>
                    <p className="app-kicker">Project Hawaii</p>
                    <h1>Dashboard</h1>
                </div>
            </header>

            <section className="dashboard-grid" aria-label="Dashboard sections">
                {dashboardCards.map((card) => (
                    <DashboardCard key={card.title} title={card.title} description={card.description} />
))}
            </section>
        </main>
    );
}
