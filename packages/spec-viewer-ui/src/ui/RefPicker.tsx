import {
  MenuToggle,
  Select,
  SelectList,
  SelectOption,
  TextInputGroup,
  TextInputGroupMain,
} from "@patternfly/react-core";
import { CodeBranchIcon } from "@patternfly/react-icons";
import { useState } from "react";
import { useIntl, type IntlShape } from "react-intl";

import type { RefList, RefName } from "../application/ports";
import { messages } from "../messages";

/** The working tree's ref in local development (see the dev server). */
export const WORKING_TREE = "@";

/** More options than this make the list slow; typing narrows it. */
const MAX_OPTIONS = 200;

export function refLabel(intl: IntlShape, ref: RefName): string {
  return ref === WORKING_TREE
    ? intl.formatMessage(messages.refWorkingTree)
    : ref;
}

interface RefPickerProps {
  refs: RefList | undefined;
  /** The selected ref; "" is the default one. */
  value: RefName;
  onChange: (ref: RefName) => void;
  /** Why the branch list is missing, when it failed to load. */
  error?: string;
}

/** Typeahead branch picker, like the fleet dashboard's scope filter. */
export function RefPicker({ refs, value, onChange, error }: RefPickerProps) {
  const intl = useIntl();
  const [isOpen, setOpen] = useState(false);
  const [input, setInput] = useState("");
  const current = value === "" ? refs?.default : value;
  const needle = input.toLowerCase();
  // The default branch leads; the others keep the server's order.
  const all = refs
    ? [refs.default, ...refs.refs.filter((ref) => ref !== refs.default)].filter(
        (ref) => refLabel(intl, ref).toLowerCase().includes(needle),
      )
    : [];
  const shown = all.slice(0, MAX_OPTIONS);
  const close = () => {
    setOpen(false);
    setInput("");
  };

  return (
    <Select
      isOpen={isOpen}
      onOpenChange={(open) => {
        if (open) setOpen(true);
        else close();
      }}
      onSelect={(_event, selected) => {
        if (typeof selected === "string") {
          // The default branch is kept as "" so links stay short.
          onChange(selected === refs?.default ? "" : selected);
        }
        close();
      }}
      // The masthead is its own stacking layer; a menu inside it would sit
      // under the page content.
      popperProps={{ appendTo: () => document.body }}
      selected={current}
      // Typing goes to the filter input, not the first option.
      shouldFocusFirstItemOnOpen={false}
      toggle={(toggleRef) => (
        <MenuToggle
          aria-label={intl.formatMessage(messages.refPickerLabel)}
          className="sv-ref-picker"
          isExpanded={isOpen}
          onClick={() => {
            if (isOpen) close();
            else setOpen(true);
          }}
          ref={toggleRef}
          variant="typeahead"
        >
          <TextInputGroup isPlain>
            <TextInputGroupMain
              aria-label={intl.formatMessage(messages.refPickerLabel)}
              autoComplete="off"
              icon={<CodeBranchIcon />}
              isExpanded={isOpen}
              onChange={(_event, text) => {
                setInput(text);
                setOpen(true);
              }}
              onClick={() => {
                setOpen(true);
              }}
              placeholder={
                current === undefined
                  ? intl.formatMessage(messages.refDefault)
                  : refLabel(intl, current)
              }
              role="combobox"
              value={input}
            />
          </TextInputGroup>
        </MenuToggle>
      )}
    >
      <SelectList isAriaMultiselectable={false}>
        {error ? (
          <SelectOption isDisabled>{error}</SelectOption>
        ) : shown.length === 0 ? (
          <SelectOption isDisabled>
            {intl.formatMessage(
              refs ? messages.refNoResults : messages.refLoading,
            )}
          </SelectOption>
        ) : (
          shown.map((ref) => (
            <SelectOption
              description={
                ref === refs?.default
                  ? intl.formatMessage(messages.refDefault)
                  : undefined
              }
              key={ref}
              value={ref}
            >
              {refLabel(intl, ref)}
            </SelectOption>
          ))
        )}
        {all.length > shown.length ? (
          <SelectOption isDisabled>
            {intl.formatMessage(messages.refMore, {
              shown: shown.length,
              total: all.length,
            })}
          </SelectOption>
        ) : null}
      </SelectList>
    </Select>
  );
}
