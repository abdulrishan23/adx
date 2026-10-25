// Data hooks for the creative screens and the public review board. These
// always call the Go server: there's no mock, because the screens upload and
// draw real files.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getJSON, patchJSON, postJSON, putJSON, uploadFile } from "./client";
import type {
  Ad,
  AdDetail,
  AdDraft,
  Board,
  BoardDecision,
  Comment,
  CommentInput,
  Concept,
  ConceptInput,
  CreativeProject,
  FileRef,
  GenerateConceptsInput,
  GeneratedAd,
  GeneratedConcepts,
  GeneratedImagery,
  GenerateImageryInput,
  GenerateResult,
  NewConcept,
  NewRender,
  Overview,
  PickInput,
  PublishInput,
  Render,
  RenderInput,
  RendersResponse,
  Settings,
  SettingsInput,
  StartedVideo,
  UploadedAd,
  UploadResult,
  VariationInput,
  VideoInput,
} from "./types.gen";

const KEY = ["creative"] as const;

export function useCreativeOverview() {
  return useQuery({
    queryKey: [...KEY, "overview"],
    queryFn: () => getJSON<Overview>("/api/creative"),
  });
}

// A project id of 0 means "not set up yet", so nothing is asked for.
export function useCreativeProject(projectId: number) {
  return useQuery({
    queryKey: [...KEY, "project", projectId],
    queryFn: () => getJSON<CreativeProject>(`/api/creative/projects/${projectId}`),
    enabled: projectId > 0,
  });
}

export function useCreativeAd(adId: number) {
  return useQuery({
    queryKey: [...KEY, "ad", adId],
    queryFn: () => getJSON<AdDetail>(`/api/creative/ads/${adId}`),
    // The editor holds its own copy while open; a refetch would fight it.
    refetchOnWindowFocus: false,
    staleTime: Number.POSITIVE_INFINITY,
  });
}

// Every creative write changes what the lists show, so they all refresh.
function useCreativeMutation<I, O>(fn: (input: I) => Promise<O>) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => qc.invalidateQueries({ queryKey: [...KEY, "overview"] }),
    onSettled: () => qc.invalidateQueries({ queryKey: [...KEY, "project"] }),
  });
}

export const useSetUpCreative = () =>
  useCreativeMutation((projectId: number) =>
    postJSON<Settings>("/api/creative/projects", { projectId }),
  );

export const useUpdateCreativeSettings = (projectId: number) =>
  useCreativeMutation((input: SettingsInput) =>
    putJSON<Settings>(`/api/creative/projects/${projectId}/settings`, input),
  );

export const useRotateReviewKey = (projectId: number) =>
  useCreativeMutation(() =>
    postJSON<Settings>(`/api/creative/projects/${projectId}/review-key`, {}),
  );

export const useSaveBrandKit = (projectId: number) =>
  useCreativeMutation((kit: unknown) =>
    putJSON<Settings>(`/api/creative/projects/${projectId}/brand-kit`, kit),
  );

export const useAddRenders = (projectId: number) =>
  useCreativeMutation((renders: NewRender[]) =>
    postJSON<RendersResponse>(`/api/creative/projects/${projectId}/renders`, { renders }),
  );

export const useUpdateRender = () =>
  useCreativeMutation(({ id, ...input }: RenderInput & { id: number }) =>
    patchJSON<Render>(`/api/creative/renders/${id}`, input),
  );

export const useCreateConcept = (projectId: number) =>
  useCreativeMutation((input: NewConcept) =>
    postJSON<Concept>(`/api/creative/projects/${projectId}/concepts`, input),
  );

// Adx writes concepts from the brand's approved facts and offers; figures
// the facts don't support are dropped and said in the notes.
export const useGenerateConcepts = (projectId: number) =>
  useCreativeMutation((input: GenerateConceptsInput) =>
    postJSON<GeneratedConcepts>(`/api/creative/projects/${projectId}/concepts/generate`, input),
  );

// Adx pictures a concept: each picture becomes a render of the project,
// labelled with the concept's code, for the generator and the editor.
export const useGenerateImagery = (conceptId: number) =>
  useCreativeMutation((input: GenerateImageryInput) =>
    postJSON<GeneratedImagery>(`/api/creative/concepts/${conceptId}/imagery`, input),
  );

// A video ad is made in the background: motion from the project's renders
// ending on the finished story ad, or a clip from the video model.
export const useStartVideo = (conceptId: number) =>
  useCreativeMutation((input: VideoInput) =>
    postJSON<StartedVideo>(`/api/creative/concepts/${conceptId}/video`, input),
  );

export const useSaveConcept = (conceptId: number) =>
  useCreativeMutation((input: ConceptInput) =>
    putJSON<Concept>(`/api/creative/concepts/${conceptId}`, input),
  );

export const useAddUploads = (projectId: number) =>
  useCreativeMutation((ads: UploadedAd[]) =>
    postJSON<UploadResult>(`/api/creative/projects/${projectId}/uploads`, { ads }),
  );

export const saveGenerated = (conceptId: number, input: GeneratedAd) =>
  postJSON<GenerateResult>(`/api/creative/concepts/${conceptId}/ads`, input);

export const saveDraft = (adId: number, input: AdDraft) =>
  putJSON<void>(`/api/creative/ads/${adId}/draft`, input);

export const publishAd = (adId: number, input: PublishInput) =>
  postJSON<Ad>(`/api/creative/ads/${adId}/versions`, input);

export const createVariation = (adId: number, input: VariationInput) =>
  postJSON<Ad>(`/api/creative/ads/${adId}/variations`, input);

export const useResolveComment = () =>
  useCreativeMutation(({ id, resolved }: { id: number; resolved: boolean }) =>
    postJSON<void>(`/api/creative/comments/${id}/resolve`, { resolved }),
  );

export const upload = (file: Blob, name?: string) => uploadFile<FileRef>("/api/files", file, name);

// Refreshes everything creative after a run of direct calls, such as
// generating a concept's ads.
export function useRefreshCreative() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: KEY });
}

// The review board, refreshed every 20 seconds while the tab is visible, as
// the old board did.
export function useReviewBoard(key: string) {
  return useQuery({
    queryKey: ["review", key],
    queryFn: () => getJSON<Board>(`/api/review/${encodeURIComponent(key)}`),
    refetchInterval: 20_000,
    refetchIntervalInBackground: false,
    retry: (count, err) => count < 2 && !("status" in err && err.status === 404),
  });
}

function useReviewMutation<I, O>(key: string, fn: (input: I) => Promise<O>) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSettled: () => qc.invalidateQueries({ queryKey: ["review", key] }),
  });
}

export const useReviewDecide = (key: string) =>
  useReviewMutation(key, (input: BoardDecision) =>
    postJSON<void>(`/api/review/${encodeURIComponent(key)}/decisions`, input),
  );

export const useReviewComment = (key: string) =>
  useReviewMutation(key, (input: CommentInput) =>
    postJSON<Comment>(`/api/review/${encodeURIComponent(key)}/comments`, input),
  );

export const useReviewPick = (key: string) =>
  useReviewMutation(key, (input: PickInput) =>
    postJSON<void>(`/api/review/${encodeURIComponent(key)}/picks`, input),
  );
