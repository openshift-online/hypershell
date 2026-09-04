"""LLM-backed step execution: a bounded tool-calling loop (AB-02 reasoning /
mechanical steps).

LangChain is imported lazily inside ``run_llm_step`` so the deterministic engine
path (tool-only steps, dry-run, gates) never imports it.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from pathlib import Path

from .types import StepSpec


@dataclass
class LLMOutcome:
    input_tokens: int = 0
    output_tokens: int = 0
    tool_calls: int = 0
    final_text: str = ""
    error: str = ""
    artifacts_written: list[str] = field(default_factory=list)


def _system_prompt(step: StepSpec) -> str:
    return (
        "You are one step in HyperShell's agentic build chain, executing the "
        "full-stack-pipeline workflow. Do exactly this step's work and nothing "
        "downstream. Use the provided tools to read and edit files and run "
        "commands in the repository. Follow existing code patterns. When the "
        "step is complete, stop calling tools and reply with a short summary of "
        "what you changed.\n\n"
        f"Step: {step.name} ({step.step_class.value})\n"
        f"Goal: {step.description}"
    )


def _task_prompt(step: StepSpec, repo_root: Path, context: str) -> str:
    parts = [f"Repository root: {repo_root}"]
    if step.consumes:
        parts.append("Inputs available from upstream steps: " + ", ".join(step.consumes))
    if context:
        parts.append("Upstream artifacts:\n" + context)
    if step.gates:
        parts.append(
            "Your work must make these acceptance commands pass (they will be "
            "run automatically after you finish): "
            + "; ".join(g.display() for g in step.gates)
        )
    parts.append("Perform the step now.")
    return "\n\n".join(parts)


def run_llm_step(
    step: StepSpec,
    model,
    tools: list,
    repo_root: Path,
    context: str = "",
    max_iters: int = 16,
) -> LLMOutcome:
    from langchain_core.messages import HumanMessage, SystemMessage, ToolMessage

    llm = model.bind_tools(tools)
    tool_by_name = {t.name: t for t in tools}
    messages = [SystemMessage(_system_prompt(step)), HumanMessage(_task_prompt(step, repo_root, context))]

    out = LLMOutcome()
    for _ in range(max_iters):
        try:
            ai = llm.invoke(messages)
        except Exception as exc:
            # Turn an API/transport error (auth, model-not-found, rate limit,
            # timeout) into a recorded step failure so the engine reports and
            # retries/escalates it instead of crashing the run with a traceback.
            out.error = f"model call failed: {type(exc).__name__}: {str(exc)[:500]}"
            return out
        usage = getattr(ai, "usage_metadata", None) or {}
        out.input_tokens += int(usage.get("input_tokens", 0) or 0)
        out.output_tokens += int(usage.get("output_tokens", 0) or 0)
        messages.append(ai)

        calls = getattr(ai, "tool_calls", None) or []
        if not calls:
            out.final_text = ai.content if isinstance(ai.content, str) else str(ai.content)
            return out
        for call in calls:
            out.tool_calls += 1
            fn = tool_by_name.get(call["name"])
            try:
                observation = fn.invoke(call["args"]) if fn else f"unknown tool {call['name']!r}"
                if fn and call["name"] == "write_file":
                    out.artifacts_written.append(str(call["args"].get("path", "")))
            except Exception as exc:  # surface tool failure back to the model
                observation = f"tool error: {exc}"
            messages.append(ToolMessage(content=str(observation), tool_call_id=call["id"]))

    out.error = f"step did not converge within {max_iters} model turns"
    return out
