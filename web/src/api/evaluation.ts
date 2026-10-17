import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON } from "./client";
import type { EvaluationReport, LabelInput, QualificationLabel } from "./types.gen";

export function useEvaluation() {
  return useQuery({
    queryKey: ["evaluation"],
    queryFn: () => getJSON<EvaluationReport>("/api/evaluation"),
  });
}

export function useLabelCall() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: LabelInput) =>
      postJSON<QualificationLabel>("/api/evaluation/labels", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["evaluation"] }),
  });
}
