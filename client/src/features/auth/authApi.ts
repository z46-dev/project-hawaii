export type AuthUser = { id: number; name: string };
export type AuthSession = { user: AuthUser | null };
export type AuthRealm = {
    id: string;
    name: string;
    type: "local" | "ldap" | "oidc" | "oauth";
    allow_registration: boolean;
};
export type Credentials = { username: string; password: string };

const currentSessionUrl = "/api/v1/auth/me";
const logoutUrl = "/api/v1/auth/logout";
const realmsUrl = "/api/v1/auth/realms";

export async function getCurrentSession() {
    const response = await fetch(currentSessionUrl, {
        credentials: "same-origin"
    });

    if (!response.ok) {
        throw new Error("Discord sign-in could not be completed.");
    }

    const payload: unknown = await response.json();
    if (!isSessionPayload(payload)) {
        throw new Error("Discord sign-in could not be completed.");
    }

    return { user: payload.user };
}

export async function logout() {
    const response = await fetch(logoutUrl, {
        method: "POST",
        credentials: "same-origin"
    });

    if (!response.ok) {
        throw new Error("Sign out could not be completed.");
    }
}

export async function getRealms() {
    const response = await fetch(realmsUrl);

    if (!response.ok) {
        throw new Error("Authentication options could not be loaded.");
    }

    const payload: unknown = await response.json();
    if (!isRealmsPayload(payload)) {
        throw new Error("Authentication options could not be loaded.");
    }

    return payload.realms;
}

export async function login(realmID: string, credentials: Credentials) {
    const response = await fetch(`/api/v1/auth/login/${encodeURIComponent(realmID)}`, {
        method: "POST",
        headers: {
            "Content-Type": "application/json"
        },
        body: JSON.stringify(credentials),
        credentials: "same-origin"
    });

    if (!response.ok) {
        throw new Error("Invalid username or password.");
    }

    return getCurrentSession();
}

export async function registerLocal(realmID: string, credentials: Credentials) {
    const response = await fetch(`/api/v1/auth/register/${encodeURIComponent(realmID)}`, {
        method: "POST",
        headers: {
            "Content-Type": "application/json"
        },
        body: JSON.stringify(credentials),
        credentials: "same-origin"
    });

    if (!response.ok) {
        if (response.status === 400) {
            throw new Error("Choose a username and a password with at least 12 characters.");
        }
        if (response.status === 409) {
            throw new Error("That username is already registered.");
        }
        throw new Error("Account registration could not be completed.");
    }

    return getCurrentSession();
}

export async function resetPassword(token: string | null, password: string) {
    if (!token) {
        throw new Error("This password reset link is invalid or has expired.");
    }
    const response = await fetch(`/api/v1/auth/password-reset/${encodeURIComponent(token)}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ password })
    });

    if (!response.ok) {
        throw new Error("This password reset link is invalid or has expired.");
    }
}

function isSessionPayload(value: unknown): value is { authenticated: true; user: AuthUser | null } {
    if (
        typeof value !== "object" ||
        value === null ||
        !("authenticated" in value) ||
        value.authenticated !== true ||
        !("user" in value)
    ) {
        return false;
    }

    return (
        value.user === null ||
        (typeof value.user === "object" &&
            value.user !== null &&
            "id" in value.user &&
            typeof value.user.id === "number" &&
            "name" in value.user &&
            typeof value.user.name === "string")
    );
}

function isRealmsPayload(value: unknown): value is { realms: AuthRealm[] } {
    return (
        typeof value === "object" &&
        value !== null &&
        "realms" in value &&
        Array.isArray(value.realms) &&
        value.realms.every(
            (realm: unknown) =>
                typeof realm === "object" &&
                realm !== null &&
                "id" in realm &&
                typeof realm.id === "string" &&
                "name" in realm &&
                typeof realm.name === "string" &&
                "type" in realm &&
                ["local", "ldap", "oidc", "oauth"].includes(String(realm.type)) &&
                "allow_registration" in realm &&
                typeof realm.allow_registration === "boolean"
        )
    );
}
