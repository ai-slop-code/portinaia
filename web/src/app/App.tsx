import { useEffect } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router-dom";

import { Workspace } from "./Workspace";

// Keep the entry component focused on application composition. Feature screens
// and workflows are composed by Workspace and their feature modules.
const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } },
});

export function App() {
  useEffect(() => {
    const clearSessionData = () => queryClient.clear();
    window.addEventListener("portinaia:session-changed", clearSessionData);
    return () =>
      window.removeEventListener("portinaia:session-changed", clearSessionData);
  }, []);

  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Workspace />
      </BrowserRouter>
    </QueryClientProvider>
  );
}
