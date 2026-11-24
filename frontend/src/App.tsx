import { BrowserRouter } from "react-router";

import { AppRouter } from "@/app/router";
import { BottomBar } from "@/components/layout/bottom-bar";
import { TopBar } from "@/components/layout/top-bar";

const App = () => {
  return (
    <BrowserRouter>
      <TopBar />
      <main className="min-h-[calc(100vh-3.5rem)] bg-background pt-14">
        <AppRouter />
      </main>
      <BottomBar />
    </BrowserRouter>
  );
};

export default App;
