const loginUrl = "/api/v1/auth/login";

export async function login(credentials) {
    const response = await fetch(loginUrl, {
        method: "POST",
        headers: {
            "Content-Type": "application/json"
        },
        body: JSON.stringify({
            username: credentials.username,
            password: credentials.password
        })
    });

    if (!response.ok) {
        throw new Error("Sign in failed. Check your username and password.");
    }

    return parseLoginResponse(await response.json());
}

function parseLoginResponse(payload) {
    if (!payload || payload.authenticated !== true) {
        throw new Error("Sign in failed. The server did not return a valid login response.");
    }

    return {
        token: payload.token ?? "",
        user: payload.user ?? null
    };
}
