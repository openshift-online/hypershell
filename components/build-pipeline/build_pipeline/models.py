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
    if provider == "openai":
        from langchain_openai import ChatOpenAI

        return ChatOpenAI(model=binding.model, **kwargs)
    if provider == "anthropic":
        from langchain_anthropic import ChatAnthropic

        return ChatAnthropic(model=binding.model, **kwargs)
    raise ValueError(
        f"unknown provider {binding.provider!r}; supported: vertex, openai, anthropic"
    )


def estimate_cost(binding: ModelBinding | None, input_tokens: int, output_tokens: int) -> float:
    """Estimate USD cost from token counts and per-million prices in params."""
    if binding is None:
        return 0.0
    p_in = float(binding.params.get("price_in_per_m", 0.0))
    p_out = float(binding.params.get("price_out_per_m", 0.0))
    return input_tokens / 1_000_000 * p_in + output_tokens / 1_000_000 * p_out
