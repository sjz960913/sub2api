import 'package:flutter/material.dart';

class PageFrame extends StatelessWidget {
  const PageFrame({
    required this.title,
    required this.child,
    this.actions = const [],
    super.key,
  });

  final String title;
  final Widget child;
  final List<Widget> actions;

  @override
  Widget build(BuildContext context) {
    return SafeArea(
      bottom: false,
      child: CustomScrollView(
        slivers: [
          SliverPadding(
            padding: const EdgeInsets.fromLTRB(24, 24, 24, 20),
            sliver: SliverToBoxAdapter(
              child: Row(
                children: [
                  Expanded(
                    child: Text(
                      title,
                      style: Theme.of(context).textTheme.headlineMedium,
                    ),
                  ),
                  ...actions,
                ],
              ),
            ),
          ),
          SliverPadding(
            padding: const EdgeInsets.fromLTRB(24, 0, 24, 28),
            sliver: SliverToBoxAdapter(child: child),
          ),
        ],
      ),
    );
  }
}
