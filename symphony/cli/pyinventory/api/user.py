#!/usr/bin/env python3
# Copyright (c) 2004-present Facebook All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

from typing import List

from ..client import SymphonyClient
from ..consts import Entity, User
from ..exceptions import EntityNotFoundError
from ..graphql.edit_user_input import EditUserInput
from ..graphql.edit_user_mutation import EditUserMutation
from ..graphql.user_query import UserQuery
from ..graphql.user_status_enum import UserStatus
from ..graphql.users_query import UsersQuery


USER_ROLE = 1


def get_user(client: SymphonyClient, email: str) -> User:
    """Returns `pyinventory.consts.User` object by its email

        Args:
            email: the email address the user registered with

        Returns:
            pyinventory.consts.User object
            
        Raises:
            EntityNotFoundError: the user was not found
            FailedOperationException: internal inventory error

        Example:
            ```
            user = client.get_user("user@test.com")
            ```
    """
    result = UserQuery.execute(client, email)
    user = result.user
    if user is None:
        raise EntityNotFoundError(entity=Entity.User, entity_name=email)
    return User(
        id=user.id,
        auth_id=user.authID,
        email=user.email,
        status=user.status,
        role=user.role,
    )


def add_user(client: SymphonyClient, email: str, password: str) -> User:
    """Adds new user to inventory with its email and password

        Args:
            email: the email address of the user
            password: password the user would connect with

        Returns:
            pyinventory.consts.User object
            
        Raises:
            EntityNotFoundError: the user was not created properly
            FailedOperationException: internal inventory error
            AssertionError: The user was not created for some known reason
            HTTPError: Error with connection

        Example:
            ```
            user = client.add_user("user@test.com", "P0ssW!rd0f43")
            ```
    """
    resp = client.post(
        "/user/async/",
        {"email": email, "password": password, "role": USER_ROLE, "networkIDs": []},
    )

    if not resp.ok:
        error_message = resp.json().get("error", None)
        if error_message is not None:
            raise AssertionError(error_message)
        raise

    return get_user(client, email)


def deactivate_user(client: SymphonyClient, user: User) -> None:
    """Deactivate the user which would prevent the user from login in to symphony
       Users in symphony are never deleted. Only de-activated.

        Args:
            user: user to deactivate
            
        Raises:
            FailedOperationException: internal inventory error

        Example:
            ```
            user = client.get_user("user@test.com")
            client.deactivate_user(user)
            ```
    """
    EditUserMutation.execute(
        client, input=EditUserInput(id=user.id, status=UserStatus.DEACTIVATED)
    )


def activate_user(client: SymphonyClient, user: User) -> None:
    """Activate the user which would allow the user to login again to symphony

        Args:
            user: user to activate
            
        Raises:
            FailedOperationException: internal inventory error

        Example:
            ```
            user = client.get_user("user@test.com")
            client.activate_user(user)
            ```
    """
    EditUserMutation.execute(
        client, input=EditUserInput(id=user.id, status=UserStatus.ACTIVE)
    )


def get_users(client: SymphonyClient) -> List[User]:
    """Get the list of users in the system (both active and deactivate)

        Returns:
            list of `pyinventory.consts.User` objects
        
        Raises:
            FailedOperationException: internal inventory error

        Example:
            ```
            users = client.get_users()
            for user in users:
                print(user.email)
            ```
    """
    result = UsersQuery.execute(client).users
    if result is None:
        return []
    users = []
    for edge in result.edges:
        node = edge.node
        if node is not None:
            users.append(
                User(
                    id=node.id,
                    auth_id=node.authID,
                    email=node.email,
                    status=node.status,
                    role=node.role,
                )
            )
    return users


def get_active_users(client: SymphonyClient) -> List[User]:
    """Get the list of the active users in the system

        Returns:
            list of `pyinventory.consts.User` objects
        
        Raises:
            FailedOperationException: internal inventory error

        Example:
            ```
            users = client.get_active_users()
            for user in users:
                print(user.email)
            ```
    """
    users = get_users(client)
    return [user for user in users if user.status == UserStatus.ACTIVE]
