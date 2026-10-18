import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON } from "./client";
import { isLive, mockResponse } from "./mock";
import { daysAgo, PROJECTS } from "./mock/data";
import type { CreateInput, Project, ProjectsResponse } from "./types.legacy";

// The project list is a real endpoint. Mocks mirror its rules (core/projects):
// a name is required, 100 characters at most, and unique.
const mockProjects: Project[] = PROJECTS.map((p, i) => ({
  id: p.id,
  name: p.name,
  createdAt: daysAgo(40 - i),
}));

function mockCreate(input: CreateInput): Project {
  const name = input.name.trim();
  if (!name) throw new Error("name is required");
  if (name.length > 100) throw new Error("name must be 100 characters or fewer");
  if (mockProjects.some((p) => p.name.toLowerCase() === name.toLowerCase()))
    throw new Error(`a project named "${name}" already exists`);
  const p = {
    id: Math.max(0, ...mockProjects.map((x) => x.id)) + 1,
    name,
    createdAt: new Date().toISOString(),
  };
  mockProjects.push(p);
  return p;
}

export function useProjects() {
  return useQuery({
    queryKey: ["projects"],
    queryFn: () =>
      isLive("projects")
        ? getJSON<ProjectsResponse>("/api/projects")
        : mockResponse((): ProjectsResponse => ({ projects: mockProjects })),
  });
}

export function useCreateProject() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateInput) =>
      isLive("projects")
        ? postJSON<Project>("/api/projects", input)
        : mockResponse(() => mockCreate(input), 300),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["projects"] }),
  });
}
