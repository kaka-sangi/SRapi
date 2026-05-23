import { Spinner } from "@/components/ui";

export default function RootLoading() {
  return (
    <div className="flex min-h-[40vh] items-center justify-center p-8">
      <Spinner size={28} label="Loading..." />
    </div>
  );
}
