import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { EvaluationReport, LabelInput, QualificationLabel } from "@/api/types.gen";
import { getJSON, postJSON } from "./client";

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
