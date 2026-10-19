// Brands, offers and facts, straight from the engine. A Brand is Reportage
// or a competitor; a development such as The Wadi is a Project under it.
// This re-exports the engine's client so our screens and the engine's share
// one cache and one set of query keys while both exist.
export {
  useAddFact,
  useAssignProject,
  useBrand,
  useBrands,
  useClaims,
  useCreateBrand,
  useCreateOffer,
  useFactAction,
  useFactHistory,
  useOffer,
  useResolveConflict,
  useReviseOffer,
  useSetOfferState,
} from "@/engine/api/brands";
