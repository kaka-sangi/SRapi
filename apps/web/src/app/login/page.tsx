import { redirect } from "next/navigation";

// `/login` is an alias for the landing page, which hosts the sign-in form.
export default function LoginPage() {
  redirect("/");
}
