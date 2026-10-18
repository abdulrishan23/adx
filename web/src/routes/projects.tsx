import type { FormEvent } from "react";
import { useCreateProject, useProjects } from "@/api/projects";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

export function ProjectsPage() {
  const projects = useProjects();
  const create = useCreateProject();

  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const name = String(new FormData(form).get("name") ?? "");
    create.mutate({ name }, { onSuccess: () => form.reset() });
  }

  return (
    <section className="space-y-6">
      <h1 className="text-3xl font-semibold tracking-tight">Projects</h1>

      <form onSubmit={onSubmit} className="flex max-w-md gap-2">
        <Input name="name" placeholder="Project name" aria-label="Project name" />
        <Button type="submit" disabled={create.isPending}>
          Add
        </Button>
      </form>
      {create.error && <p className="text-sm text-destructive">{create.error.message}</p>}

      {projects.isPending && <p className="text-muted-foreground">Loading…</p>}
      {projects.error && <p className="text-destructive">{projects.error.message}</p>}
      {projects.data && (
        <ul className="divide-y border-y">
          {projects.data.projects.map((p) => (
            <li key={p.id} className="py-3">
              {p.name}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
