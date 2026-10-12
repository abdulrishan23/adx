import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON } from "./client";
import type { CreateInput, Project, ProjectsResponse } from "./types.gen";

export function useProjects() {
  return useQuery({
    queryKey: ["projects"],
    queryFn: () => getJSON<ProjectsResponse>("/api/projects"),
  });
}

export function useCreateProject() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateInput) => postJSON<Project>("/api/projects", input),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["projects"] }),
  });
}
