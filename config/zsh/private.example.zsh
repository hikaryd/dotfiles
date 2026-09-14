# Личные настройки: ~/.config/dots/private.zsh; заполните свои значения.
# Храните вне Git, права файла — 600.

# Необязательный личный шлюз API.
# export OPENAI_BASE_URL="https://gateway.example/v1/account/gateway/openai"

export KUBE_DEV_KUBECONFIG="$HOME/.kube/configs/dev.yaml"
export KUBE_DEV_CONTEXT="dev-context"
export KUBE_DEV_NAMESPACE="example-dev"

export KUBE_STAGE_KUBECONFIG="$HOME/.kube/configs/stage.yaml"
export KUBE_STAGE_CONTEXT="stage-context"
export KUBE_STAGE_NAMESPACE="example-stage"

export KUBE_PREPROD_KUBECONFIG="$HOME/.kube/configs/preprod.yaml"
export KUBE_PREPROD_CONTEXT="preprod-context"
export KUBE_PREPROD_NAMESPACE="example-preprod"

export KUBE_PROD_KUBECONFIG="$HOME/.kube/configs/prod.yaml"
export KUBE_PROD_CONTEXT="prod-context"
export KUBE_PROD_NAMESPACE="example-prod"
