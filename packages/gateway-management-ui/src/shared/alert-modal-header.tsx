import { ModalHeader, type ModalHeaderProps } from "@patternfly/react-core";

type AlertModalHeaderProps = Omit<ModalHeaderProps, "titleIconVariant"> & {
  titleIconVariant?: NonNullable<ModalHeaderProps["titleIconVariant"]>;
};

export function AlertModalHeader({
  titleIconVariant = "warning",
  ...props
}: AlertModalHeaderProps) {
  return <ModalHeader {...props} titleIconVariant={titleIconVariant} />;
}
