import { afterEach, describe, expect, it, vi } from "vitest";

import { clearAuthSession, getAuthSession, hasAuthSession, saveAuthSession } from "./authSession";

function createSessionStorage() {
    const values = new Map<string, string>();
    return {
        getItem: (key: string) => values.get(key) ?? null,
        setItem: (key: string, value: string) => values.set(key, value),
        removeItem: (key: string) => values.delete(key)
    };
}

afterEach(() => vi.unstubAllGlobals());

describe("auth session storage", () => {
    it("round trips a session", () => {
        vi.stubGlobal("sessionStorage", createSessionStorage());
        const session = { user: { id: 7, name: "Kai" } };

        saveAuthSession(session);

        expect(getAuthSession()).toEqual(session);
        expect(hasAuthSession()).toBe(true);
    });

    it("clears malformed stored data", () => {
        const storage = createSessionStorage();
        vi.stubGlobal("sessionStorage", storage);
        storage.setItem("project-hawaii.auth-session", JSON.stringify({ user: { id: "bad", name: "Kai" } }));

        expect(getAuthSession()).toBeNull();
        expect(storage.getItem("project-hawaii.auth-session")).toBeNull();
    });

    it("clears a saved session", () => {
        vi.stubGlobal("sessionStorage", createSessionStorage());
        saveAuthSession({ user: null });

        clearAuthSession();

        expect(getAuthSession()).toBeNull();
    });
});
