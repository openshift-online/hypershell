"""Construct a LangChain chat model from a resolved ModelBinding (AB-03).

Provider libraries are imported lazily so importing this module -- and the whole
package -- never requires LangChain to be installed. Only a real (non-dry-run)
LLM step touches these imports. A tool-only step resolves to ``None`` and never
constructs a model.
"""

from __future__ import annotations

from .types import ModelBinding

# Keys carried in a binding's params for cost estimation, not model kwargs.
_PRICING_KEYS = ("price_in_per_m", "price_out_per_m")


def _model_kwargs(binding: ModelBinding) -> dict:
    return {k: v for k, v in binding.params.items() if k not in _PRICING_KEYS}


def build_chat_model(binding: ModelBinding | None):
    """Return a LangChain ``BaseChatModel`` for the binding, or ``None``."""
    if binding is None:
        return None
    provider = binding.provider.lower()
    kwargs = _model_kwargs(binding)
    if provider in ("vertex", "vertexai", "google-vertex", "google_vertexai"):
        from langchain_google_vertexai import ChatVertexAI

        return ChatVertexAI(model=binding.model, **kwargs)
    if provider in ("anthropic-vertex", "anthropic_vertex", "vertex-anthropic", "claude-vertex"):
        # Claude via Vertex AI Model Garden. LangChain does NOT read
        # ANTHROPIC_VERTEX_PROJECT_ID / CLOUD_ML_REGION, so resolve them here:
        # profile params win, then those env vars, then the GOOGLE_* fallbacks.
        import os

        from langchain_google_vertexai.model_garden import ChatAnthropicVertex

        project = (
            kwargs.pop("project", None)
            or os.getenv("ANTHROPIC_VERTEX_PROJECT_ID")
            or os.getenv("GOOGLE_CLOUD_PROJECT")
        )
        location = (
            kwargs.pop("location", None)
            or os.getenv("CLOUD_ML_REGION")
            or os.getenv("GOOGLE_CLOUD_LOCATION")
            or "us-central1"
        )
        if not project:
            raise ValueError(
                "anthropic-vertex needs a GCP project: set ANTHROPIC_VERTEX_PROJECT_ID "
                "or GOOGLE_CLOUD_PROJECT, or add project=... to the profile tier params"
            )
        return ChatAnthropicVertex(model=binding.model, project=project, location=location, **kwargs)
    if provider == "openai":
        from langchain_openai import ChatOpenAI

        return ChatOpenAI(model=binding.model, **kwargs)
    if provider == "anthropic":
        from langchain_anthropic import ChatAnthropic

        return ChatAnthropic(model=binding.model, **kwargs)
    raise ValueError(
        f"unknown provider {binding.provider!r}; supported: vertex, anthropic-vertex, "
        "openai, anthropic"
    )


def check_credentials(binding: ModelBinding) -> str | None:
    """Return None if credentials for this binding appear present; an error
    string if something is clearly missing. Makes no API calls."""
    import os
    from pathlib import Path

    provider = binding.provider.lower()

    def _adc_missing() -> bool:
        adc = Path.home() / ".config" / "gcloud" / "application_default_credentials.json"
        return not adc.exists()

    if provider in ("anthropic-vertex", "anthropic_vertex", "vertex-anthropic", "claude-vertex"):
        project = (
            binding.params.get("project")
            or os.getenv("ANTHROPIC_VERTEX_PROJECT_ID")
            or os.getenv("GOOGLE_CLOUD_PROJECT")
        )
        if not project:
            return (
                "anthropic-vertex requires a GCP project: set ANTHROPIC_VERTEX_PROJECT_ID "
                "or GOOGLE_CLOUD_PROJECT, or add project=... to the profile tier params\n"
                "  sandbox alternative: set ANTHROPIC_BASE_URL=https://inference.local "
                "and use --profile sandbox"
            )
        key_file = os.getenv("GOOGLE_APPLICATION_CREDENTIALS")
        if key_file and not Path(key_file).exists():
            return f"GOOGLE_APPLICATION_CREDENTIALS={key_file!r} does not exist"
        if not key_file and _adc_missing():
            return (
                "no GCP credentials found: run 'gcloud auth application-default login' "
                "or set GOOGLE_APPLICATION_CREDENTIALS\n"
                "  sandbox alternative: set ANTHROPIC_BASE_URL=https://inference.local "
                "and use --profile sandbox"
            )
        return None

    if provider in ("vertex", "vertexai", "google-vertex", "google_vertexai"):
        key_file = os.getenv("GOOGLE_APPLICATION_CREDENTIALS")
        if key_file and not Path(key_file).exists():
            return f"GOOGLE_APPLICATION_CREDENTIALS={key_file!r} does not exist"
        if not key_file and _adc_missing():
            return (
                "no GCP credentials found: run 'gcloud auth application-default login' "
                "or set GOOGLE_APPLICATION_CREDENTIALS"
            )
        return None

    if provider == "anthropic":
        if not os.getenv("ANTHROPIC_API_KEY") and not os.getenv("ANTHROPIC_BASE_URL"):
            return (
                "anthropic provider requires ANTHROPIC_API_KEY or ANTHROPIC_BASE_URL; "
                "for the HyperShell inference router: "
                "export ANTHROPIC_BASE_URL=https://inference.local"
            )
        return None

    if provider == "openai":
        if not os.getenv("OPENAI_API_KEY"):
            return "openai provider requires OPENAI_API_KEY"
        return None

    return None  # unknown provider: let it fail at call time


def estimate_cost(binding: ModelBinding | None, input_tokens: int, output_tokens: int) -> float:
    """Estimate USD cost from token counts and per-million prices in params."""
    if binding is None:
        return 0.0
    p_in = float(binding.params.get("price_in_per_m", 0.0))
    p_out = float(binding.params.get("price_out_per_m", 0.0))
    return input_tokens / 1_000_000 * p_in + output_tokens / 1_000_000 * p_out
