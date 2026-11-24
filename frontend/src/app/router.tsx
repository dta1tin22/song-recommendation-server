import { Navigate, Route, Routes } from "react-router";

import { LoginPage, SignupPage } from "@/features/auth";
import { SearchPage } from "@/features/search";
import { HomePage } from "@/features/home";

import { PATHS } from "./routes/paths";

export function AppRouter() {
  return (
    <Routes>
      <Route path={PATHS.home} element={<HomePage />} />
      <Route path={PATHS.login} element={<LoginPage />} />
      <Route path={PATHS.signup} element={<SignupPage />} />
      <Route path={PATHS.search} element={<SearchPage />} />
      <Route path="*" element={<Navigate to={PATHS.home} replace />} />
    </Routes>
  );
}
