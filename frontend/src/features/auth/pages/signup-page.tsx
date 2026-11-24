import { SignupForm } from "../components/signup-form";

export function SignupPage() {
  return (
    <div className="flex min-h-[calc(100vh-8rem)] items-center justify-center px-4 py-12 sm:py-16">
      <SignupForm />
    </div>
  );
}
