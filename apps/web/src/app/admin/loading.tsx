import { Spinner } from "@/components/ui/spinner";

export default function AdminLoading() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Spinner className="size-5" />
    </div>
  );
}
