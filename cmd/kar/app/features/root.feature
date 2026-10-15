Feature: Root command initialization
  The kar root command creates, waits for and deletes runner resources

  Scenario Outline: initialization process
    Given a mock runner
    When the root command is executed with flag "<flag>" value "<value>" and induced failure "<failure>"
    Then the command execution should "<outcome>"

    Examples:
      | flag | value       | failure       | outcome |
      |      |             | none          | succeed |
      | -c   | test config | none          | succeed |
      | -t   | vm template | none          | succeed |
      | -n   | kubevirt    | none          | succeed |
      | -r   | runner name | none          | succeed |
      |      |             | create        | fail    |
      |      |             | delete        | fail    |
      |      |             | wait          | fail    |
      |      |             | runner failed | fail    |

  Scenario Outline: created resources are deleted exactly once on every exit path
    Given a mock runner
    When the root command is executed with flag "" value "" and induced failure "<failure>"
    Then DeleteResources should be called <calls> times
    And the command execution should "<outcome>"

    Examples:
      | failure       | calls | outcome |
      | none          | 1     | succeed |
      | wait          | 1     | fail    |
      | runner failed | 1     | fail    |
      | delete        | 1     | fail    |
      | create        | 0     | fail    |

  Scenario: a termination signal during the wait still deletes the resources
    Given a mock runner whose wait is interrupted by a termination signal
    When the root command is executed with flag "" value "" and induced failure "none"
    Then DeleteResources should be called 1 time
    And DeleteResources should receive a live context with a deadline

  Scenario: a zero cleanup timeout does not bound the deletion
    Given a mock runner without a cleanup timeout
    When the root command is executed with flag "" value "" and induced failure "none"
    Then DeleteResources should receive a context without a deadline
