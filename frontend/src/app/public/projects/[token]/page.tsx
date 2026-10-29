import { Suspense } from "react";
import { PublicProjectPage } from "@/modules/projects/pages/PublicProjectPage";

export default async function Page({ params }: { params: Promise<{ token: string }> }) {
  const { token } = await params;
  return (
    <Suspense fallback={null}>
      <PublicProjectPage token={token} />
    </Suspense>
  );
}
