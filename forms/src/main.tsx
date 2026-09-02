// Entry point: TanStack Router (code-based routes; the app has exactly one
// real route) + TanStack Query, mounted under the shell the Go forms service
// serves at /f/<publicId>.

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createRootRoute, createRoute, createRouter, Outlet, RouterProvider } from "@tanstack/react-router";

import { FormPage } from "./FormPage";
import { NotFound } from "./NotFound";
import "./styles.css";

const rootRoute = createRootRoute({
    component: () => <Outlet />,
    notFoundComponent: NotFound,
});

const formRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/f/$publicId",
    component: FormPage,
});

const router = createRouter({ routeTree: rootRoute.addChildren([formRoute]) });

declare module "@tanstack/react-router" {
    interface Register {
        router: typeof router;
    }
}

const queryClient = new QueryClient();

createRoot(document.getElementById("root")!).render(
    <StrictMode>
        <QueryClientProvider client={queryClient}>
            <RouterProvider router={router} />
        </QueryClientProvider>
    </StrictMode>,
);
