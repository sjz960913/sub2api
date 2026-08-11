import 'package:flutter/material.dart';

import '../../app/theme.dart';

class AppIconTile extends StatelessWidget {
  const AppIconTile(this.icon, {this.color = AppColors.primary, super.key});

  final IconData icon;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 46,
      height: 46,
      decoration: BoxDecoration(
        color: AppColors.iconTile,
        borderRadius: BorderRadius.circular(13),
      ),
      alignment: Alignment.center,
      child: Icon(icon, color: color),
    );
  }
}
