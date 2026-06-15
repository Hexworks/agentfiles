---
id: 0002
type: feature
status: wont-do
depends_on: 0014
superseded_by: 0015
---

> [!IMPORTANT]
> This task depends on #0014 that creates the wizard component.

# Use wizards for dynamic forms

Currently we have forms on the TUI so for example if the new asset creation option is selected we see all form fields at once.

The problem with this is that it adds complexity (how do I move between fields for example) but it also prevents dynamism. One such example is that we can't change how the form looks like based on the type of asset the user selects.

What we need instead is a wizard-like behavior. When an action is invoked only a single field is presented to the user, and the next field is only rendered after the user is done with the previous one.

All forms need to be refactored to wizards, and the fields that we ask need to be in the right order:

## Create Profile

1. Name
2. Path

## Init Asset

1. Type
2. Id
3. Name
4. Description

## Add Project

1. Name
2. Path (can we use a path selector component?)
3. Agents
4. Assets

Note that _huh_ already supports this use case ([conditional forms](https://github.com/charmbracelet/huh/blob/main/examples/conditional/main.go)).
