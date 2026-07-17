const authSessionKey = "project-hawaii.auth-session";

export function hasAuthSession() {
    return getAuthSession() !== null;
}

export function getAuthSession() {
    try {
        return JSON.parse(sessionStorage.getItem(authSessionKey));
    } catch {
        clearAuthSession();
        return null;
    }
}

export function saveAuthSession(session) {
    sessionStorage.setItem(authSessionKey, JSON.stringify(session));
}

export function clearAuthSession() {
    sessionStorage.removeItem(authSessionKey);
}
