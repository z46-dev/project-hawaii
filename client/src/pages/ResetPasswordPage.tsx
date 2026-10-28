import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";

import { buttonVariants } from "@/components/ui/button";
import { resetPassword } from "../features/auth/authApi";
import type { FormEvent } from "react";

export function ResetPasswordPage() {
    const navigate = useNavigate();
    const [errorMessage, setErrorMessage] = useState("");

    async function handleSubmit(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        const password = String(new FormData(event.currentTarget).get("password") ?? "");
        const token = new URLSearchParams(window.location.search).get("token");

        try {
            await resetPassword(token, password);
            navigate({ to: "/login" });
        } catch (error) {
            setErrorMessage(error instanceof Error ? error.message : "Something went wrong.");
        }
    }

    return (
        <main className="auth-shell">
            <section className="auth-panel">
                <h1>Set your password</h1>
                <form className="login-form" onSubmit={handleSubmit}>
                    <label className="field">
                        <span>New password</span>
                        <input name="password" type="password" minLength={12} autoComplete="new-password" required />
                    </label>
                    <button className={buttonVariants({ size: "lg", className: "h-11 w-full" })} type="submit">
                        Save password
                    </button>
                    {errorMessage ? (
                        <p className="form-note form-note-error" role="alert">
                            {errorMessage}
                        </p>
                    ) : null}
                </form>
            </section>
        </main>
    );
}
