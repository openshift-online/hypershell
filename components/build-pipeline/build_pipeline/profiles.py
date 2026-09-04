"""Profile loading and per-step model resolution (AB-03).

A profile is a TOML file that binds each tier to a concrete model and may
override the model (by tier) for individual steps. Resolution order for a
step's model is: per-step override -> the step class's default tier -> the
profile's binding for that tier. Tool-only steps resolve to ``None`` (no model)
regardless of profile.
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

try:  # Python 3.11+
    import tomllib
except ModuleNotFoundError:  # pragma: no cover - fallback for 3.10
    import tomli as tomllib  # type: ignore

from .types import ModelBinding, StepSpec, Tier

BUILTIN_DIR = Path(__file__).parent / "profiles"


@dataclass(frozen=True)
class Profile:
    name: str
    # tier name -> {"provider": str, "model": str, "params": dict}
    tiers: dict[str, dict]
    # step id -> tier name (override of the class default tier)
    overrides: dict[str, str]

    def resolve(self, step: StepSpec) -> ModelBinding | None:
        tier_name = self.overrides.get(step.id, step.default_tier().value)
        tier = Tier(tier_name)
        if tier is Tier.NONE:
            return None
        cfg = self.tiers.get(tier.value)
        if cfg is None:
            raise KeyError(
                f"profile {self.name!r} defines no model for tier {tier.value!r} "
                f"(needed by step {step.id!r})"
            )
        return ModelBinding(
            tier=tier,
            provider=cfg["provider"],
            model=cfg["model"],
            params=dict(cfg.get("params", {})),
        )


def _profile_path(name_or_path: str) -> Path:
    p = Path(name_or_path)
    if p.suffix == ".toml" and p.exists():
        return p
    builtin = BUILTIN_DIR / f"{name_or_path}.toml"
    if builtin.exists():
        return builtin
    raise FileNotFoundError(
        f"no profile {name_or_path!r} (looked for a .toml path and "
        f"{builtin})"
    )


def load_profile(name_or_path: str) -> Profile:
    path = _profile_path(name_or_path)
    with path.open("rb") as fh:
        data = tomllib.load(fh)
    tiers = data.get("tiers", {})
    if not tiers:
        raise ValueError(f"profile {path} defines no [tiers.*] bindings")
    return Profile(
        name=data.get("name", path.stem),
        tiers=tiers,
        overrides=data.get("overrides", {}),
    )


def list_profiles() -> list[str]:
    return sorted(p.stem for p in BUILTIN_DIR.glob("*.toml"))
