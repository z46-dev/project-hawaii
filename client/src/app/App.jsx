import { RouterProvider } from "@tanstack/react-router";

import { AppProviders } from "./AppProviders.jsx";
import { router } from "../routes/router.jsx";

export function App() {
    return (
        <AppProviders>
            <RouterProvider router={router} />
        </AppProviders>
    );
}
