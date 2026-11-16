import type { FormEvent } from "react";
import { Card, CardContent } from "@mui/material";
import type { components } from "../../api/generated";
import { ProviderCreateForm } from "./ProviderCreateForm";
import { ProviderDetails } from "./ProviderDetails";

type Provider = components["schemas"]["Provider"];

type Props = {
  selected: Provider | null;
  editing: boolean;
  onCreate: (event: FormEvent<HTMLFormElement>) => void;
  onUpdate: (event: FormEvent<HTMLFormElement>) => void;
  onToggleEdit: () => void;
  onLifecycle: () => void;
};

export function ProviderWorkspace(props: Props) {
  return (
    <div data-testid="provider-workspace">
      <ProviderCreateForm onSubmit={props.onCreate} />
      {props.selected && (
        <Card>
          <CardContent>
            <ProviderDetails
              provider={props.selected}
              editing={props.editing}
              onToggleEdit={props.onToggleEdit}
              onLifecycle={props.onLifecycle}
              onUpdate={props.onUpdate}
            />
          </CardContent>
        </Card>
      )}
    </div>
  );
}
