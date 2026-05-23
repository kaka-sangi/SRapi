import Link from "next/link";
import { Button, Card, CardDescription, CardTitle } from "@/components/ui";

export default function NotFound() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center p-8">
      <Card className="max-w-lg space-y-6 text-center">
        <div className="space-y-2">
          <CardTitle>Page not found</CardTitle>
          <CardDescription>
            The page you opened does not exist in this SRapi build. Use the link below to head back.
          </CardDescription>
        </div>
        <div className="flex justify-center">
          <Button asChild>
            <Link href="/">Back to home</Link>
          </Button>
        </div>
      </Card>
    </div>
  );
}
