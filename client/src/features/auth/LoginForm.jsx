import { LogIn } from "lucide-react";
import { useNavigate } from "@tanstack/react-router";
import { useMutation } from "@tanstack/react-query";

import { login } from "./authApi.js";
import { saveAuthSession } from "./authSession.js";

export function LoginForm() {
    const navigate = useNavigate();
    const loginMutation = useMutation({
        mutationFn: login,
        onSuccess: (session) => {
            saveAuthSession(session);
            navigate({ to: "/dashboard" });
        }
    });

    function handleSubmit(event) {
        event.preventDefault();
        loginMutation.mutate(Object.fromEntries(new FormData(event.currentTarget)));
    }

    return (
        <form className="login-form" onSubmit={handleSubmit}>
            <label className="field">
                <span>Username</span>
                <input name="username" type="text" autoComplete="username" required />
            </label>

            <label className="field">
                <span>Password</span>
                <input name="password" type="password" autoComplete="current-password" required />
            </label>

            <button className="button button-primary" type="submit" disabled={loginMutation.isPending}>
                <LogIn size={18} aria-hidden="true" />
                {loginMutation.isPending ? "Signing in" : "Sign in"}
            </button>

            {loginMutation.isError ? (
                <p className="form-note form-note-error" role="alert">
                    {loginMutation.error.message}
                </p>
            ) : null}
        </form>
    );
}
