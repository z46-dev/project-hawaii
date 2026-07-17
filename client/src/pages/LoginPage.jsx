import { BrandMark } from "../components/BrandMark.jsx";
import { LoginForm } from "../features/auth/LoginForm.jsx";

export function LoginPage() {
    return (
        <main className="auth-shell">
            <section className="auth-panel" aria-labelledby="login-heading">
                <div className="auth-header">
                    <BrandMark />
                    <div>
                        <p className="app-kicker">Project Hawaii</p>
                        <h1 id="login-heading">Sign in</h1>
                    </div>
                </div>

                <LoginForm />
            </section>
        </main>
    );
}
