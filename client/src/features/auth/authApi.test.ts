import { afterEach, describe, expect, it, vi } from "vitest";

import { getCurrentSession, getRealms } from "./authApi";

afterEach(() => vi.unstubAllGlobals());

describe("auth API", () => {
    it("returns a validated current session", async () => {
        vi.stubGlobal(
            "fetch",
            vi.fn().mockResolvedValue(Response.json({ authenticated: true, user: { id: 7, name: "Kai" } }))
        );

        await expect(getCurrentSession()).resolves.toEqual({ user: { id: 7, name: "Kai" } });
    });

    it("rejects malformed current session responses", async () => {
        vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json({ authenticated: true, user: { id: "7" } })));

        await expect(getCurrentSession()).rejects.toThrow("Discord sign-in could not be completed.");
    });

    it("returns realms from a valid response", async () => {
        const realms = [{ id: "staff", name: "Staff", type: "ldap", allow_registration: false }];
        vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json({ realms })));

        await expect(getRealms()).resolves.toEqual(realms);
    });

    it("rejects a malformed realms response", async () => {
        vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json({ realms: [{ id: "staff" }] })));

        await expect(getRealms()).rejects.toThrow("Authentication options could not be loaded.");
    });
});
