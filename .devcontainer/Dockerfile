FROM mcr.microsoft.com/devcontainers/base:noble

RUN apt-get update && export DEBIAN_FRONTEND=noninteractive \
    # Restore man command
    # && yes | unminimize 2>&1 \
    && apt-get upgrade -y \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        dnsutils \
        fish \
    && rm -rf /var/lib/apt/lists/* \
    && apt-get autoremove -y \
    && apt-get clean -y

ARG USERNAME=vscode
ENV HOME=/home/${USERNAME}
USER ${USERNAME}
WORKDIR ${HOME}

# Install mise to manage the tools needed for this project
RUN curl -sS https://mise.run | sh > /dev/null
ENV PATH=${HOME}/.local/bin/:${PATH}

# Make sure mise is active for the common shells, e.g. installed tools are in PATH
RUN echo 'eval "$($HOME/.local/bin/mise activate bash)"' >> ~/.bashrc
RUN echo 'eval "$($HOME/.local/bin/mise activate zsh)"' >> ~/.zshrc
RUN mkdir -p ~/.config/fish/conf.d && echo '$HOME/.local/bin/mise activate fish | source' >> ~/.config/fish/conf.d/mise.fish

# Copying so global install included in docker build, but also mounting in
# devcontainer.json so user changes persist
COPY --chown=${USERNAME}:${USERNAME} config/mise-global.toml ${HOME}/.config/mise/config.toml
RUN ${HOME}/.local/bin/mise -y install

# The oh-my-posh prompt engine, themes at https://ohmyposh.dev/docs/themes
RUN curl -sS https://ohmyposh.dev/install.sh | bash -s > /dev/null

# Try to set the user's desired login shell - needs to be done at build time so
# that vscode auto detects it for terminals.  Set USER_SHELL to
# ${localEnv:SHELL} in devcontainer.json build.args
ARG USER_SHELL=bash
RUN BASE_USER_SHELL=$(basename ${USER_SHELL}); \
    BASE_IMAGE_USER_SHELL=$(getent passwd ${USERNAME} | cut -d: -f7 | xargs basename); \
    if [ "${BASE_USER_SHELL}" != "${BASE_IMAGE_USER_SHELL}" ]; then \
        USER_SHELL=$(which ${BASE_USER_SHELL}); \
        if [ -x "${USER_SHELL}" ]; then \
            echo "Changing login shell to ${USER_SHELL}"; \
            sudo usermod --shell "${USER_SHELL}" ${USERNAME}; \
        else \
            echo "The login shell '${USER_SHELL}' does not exist"; \
        fi \
    fi
