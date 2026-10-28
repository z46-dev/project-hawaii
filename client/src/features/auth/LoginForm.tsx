import { useNavigate } from "@tanstack/react-router";
import { useEffect, useState } from "react";

import { getCurrentSession, getRealms, login, registerLocal, type AuthRealm } from "./authApi";
import { saveAuthSession } from "./authSession";
import { buttonVariants } from "@/components/ui/button";
import type { FormEvent } from "react";

export function LoginForm() {
    const navigate = useNavigate();
    const [errorMessage, setErrorMessage] = useState("");
    const [realms, setRealms] = useState<AuthRealm[]>([]);
    const [realmID, setRealmID] = useState("");
    const [isLoading, setIsLoading] = useState(true);
    const [isRegistering, setIsRegistering] = useState(false);

    const realm = realms.find((item) => item.id === realmID);
    const canRegister = realm?.type === "local" && realm.allow_registration;

    useEffect(() => {
        getRealms()
            .then((items) => {
                setRealms(items);
                setRealmID(items[0]?.id ?? "");
            })
            .catch((error: unknown) => setErrorMessage(getErrorMessage(error)))
            .finally(() => setIsLoading(false));
    }, [navigate]);

    useEffect(() => {
        if (new URLSearchParams(window.location.search).get("auth") !== "success") {
            return;
        }

        getCurrentSession()
            .then((session) => {
                saveAuthSession(session);
                navigate({ to: "/dashboard" });
            })
            .catch((error: unknown) => setErrorMessage(getErrorMessage(error)));
    }, [navigate]);

    async function handleSubmit(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        setErrorMessage("");

        try {
            const formData = new FormData(event.currentTarget);
            const credentials = {
                username: String(formData.get("username") ?? ""),
                password: String(formData.get("password") ?? "")
            };
            const session = isRegistering
                ? await registerLocal(realmID, credentials)
                : await login(realmID, credentials);
            saveAuthSession(session);
            navigate({ to: "/dashboard" });
        } catch (error) {
            setErrorMessage(getErrorMessage(error));
        }
    }

    return (
        <div className="login-form">
            <label className="field">
                <span>Sign in with</span>
                <select
                    className="border-input bg-background focus-visible:ring-ring h-11 w-full rounded-md border px-3 text-sm shadow-xs focus-visible:ring-2"
                    value={realmID}
                    onChange={(event) => {
                        setRealmID(event.target.value);
                        setIsRegistering(false);
                        setErrorMessage("");
                    }}
                    disabled={isLoading}
                >
                    {realms.map((item) => (
                        <option key={item.id} value={item.id}>
                            {item.name}
                        </option>
                    ))}
                </select>
            </label>

            {realm?.type === "oidc" || realm?.type === "oauth" ? (
                <a
                    className={buttonVariants({ size: "lg", className: "h-11 w-full" })}
                    href={`/api/v1/auth/${realm.type}/${encodeURIComponent(realm.id)}`}
                >
                    Continue with {realm.name}
                </a>
            ) : (
                <form className="login-form" onSubmit={handleSubmit}>
                    <label className="field">
                        <span>Username</span>
                        <input name="username" required />
                    </label>
                    <label className="field">
                        <span>Password</span>
                        <input
                            name="password"
                            type="password"
                            autoComplete={isRegistering ? "new-password" : "current-password"}
                            minLength={isRegistering ? 12 : undefined}
                            required
                        />
                    </label>
                    <button
                        className={buttonVariants({ size: "lg", className: "h-11 w-full" })}
                        type="submit"
                        disabled={!realmID}
                    >
                        {isRegistering ? "Create account" : "Sign in"}
                    </button>
                </form>
            )}

            {canRegister ? (
                <p className="form-note">
                    {isRegistering ? "Already have an account? " : "Need an account? "}
                    <button
                        className="text-primary underline underline-offset-4"
                        type="button"
                        onClick={() => {
                            setIsRegistering(!isRegistering);
                            setErrorMessage("");
                        }}
                    >
                        {isRegistering ? "Sign in" : "Create account"}
                    </button>
                </p>
            ) : null}

            {errorMessage ? (
                <p className="form-note form-note-error" role="alert">
                    {errorMessage}
                </p>
            ) : null}
        </div>
    );
}

function getErrorMessage(error: unknown) {
    return error instanceof Error ? error.message : "Something went wrong.";
}
