import type { AuthSession } from "./authApi";

const authSessionKey = "project-hawaii.auth-session";

export function hasAuthSession() {
    return getAuthSession() !== null;
}

export function getAuthSession() {
    try {
        const storedSession: unknown = JSON.parse(sessionStorage.getItem(authSessionKey) ?? "null");
        if (isAuthSession(storedSession)) {
            return storedSession;
        }

        clearAuthSession();
        return null;
    } catch {
        clearAuthSession();
        return null;
    }
}

export function saveAuthSession(session: AuthSession) {
    sessionStorage.setItem(authSessionKey, JSON.stringify(session));
}

function isAuthSession(value: unknown): value is AuthSession {
    return (
        typeof value === "object" &&
        value !== null &&
        "user" in value &&
        (value.user === null ||
            (typeof value.user === "object" &&
                value.user !== null &&
                "id" in value.user &&
                typeof value.user.id === "number" &&
                "name" in value.user &&
                typeof value.user.name === "string"))
    );
}

export function clearAuthSession() {
    sessionStorage.removeItem(authSessionKey);
}
