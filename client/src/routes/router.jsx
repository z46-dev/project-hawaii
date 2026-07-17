import { createRootRoute, createRoute, createRouter, Outlet, redirect } from "@tanstack/react-router";

import { hasAuthSession } from "../features/auth/authSession.js";
import { DashboardPage } from "../pages/DashboardPage.jsx";
import { LoginPage } from "../pages/LoginPage.jsx";

const rootRoute = createRootRoute({
    component: RootLayout
});

const indexRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/",
    beforeLoad: () => {
        if (hasAuthSession()) {
            throw redirect({ to: "/dashboard" });
        }
    },
    component: LoginPage
});

const loginRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/login",
    beforeLoad: () => {
        throw redirect({ to: "/" });
    }
});

const dashboardRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/dashboard",
    beforeLoad: () => {
        if (!hasAuthSession()) {
            throw redirect({ to: "/" });
        }
    },
    component: DashboardPage
});

const routeTree = rootRoute.addChildren([indexRoute, loginRoute, dashboardRoute]);

export const router = createRouter({ routeTree });

function RootLayout() {
    return <Outlet />;
}
