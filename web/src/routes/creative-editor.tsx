import { lazy, Suspense } from "react";
import { useCreativeAd } from "@/api/creative";
import { useSession } from "@/app/session";
import { ErrorNote, LoadingRows } from "@/components/page";

// The canvas library is large, so the editor loads only when it's opened.
const Editor = lazy(() => import("@/creative/editor").then((m) => ({ default: m.Editor })));

export function CreativeEditorPage({ adId }: { adId: number }) {
  const detail = useCreativeAd(adId);
  const session = useSession();
  if (session.role === "viewer") {
    return <ErrorNote error={new Error("Viewers can see ads but not edit them.")} />;
  }
  if (detail.isPending) return <LoadingRows rows={10} />;
  if (detail.error) return <ErrorNote error={detail.error} />;
  return (
    <Suspense fallback={<p className="text-sm text-muted-foreground">Starting the editor…</p>}>
      <Editor key={adId} detail={detail.data} />
    </Suspense>
  );
}
